package proxmoxsetup

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/eyupio/zoomies/internal/installer"
	"github.com/eyupio/zoomies/internal/version"
)

const TemplateName = "zoomies-template"

// DefaultBridge is the bridge a stock Proxmox install creates, and the one a
// guest is attached to when the node has it and nothing else is asked for.
const DefaultBridge = "vmbr0"

// ActiveBridge names the bridge a guest on this node should attach to: vmbr0
// when it is up, otherwise the first active bridge. It is read from the node
// itself, as root, so it does not depend on what an API token may list. The
// template is built on it and the provider is saved with it, so a node whose
// bridge is not vmbr0 needs nothing typed by hand.
func ActiveBridge(ctx context.Context, h Host, node string) (string, error) {
	raw, err := h.Run(ctx, "pvesh", "get", "/nodes/"+node+"/network", "--output-format", "json")
	if err != nil {
		return "", errors.New("proxmox setup: pvesh failed; correct the host configuration and rerun setup")
	}
	var networks []struct {
		Interface string `json:"iface"`
		Type      string `json:"type"`
		Active    int    `json:"active"`
	}
	if json.Unmarshal(raw, &networks) != nil {
		return "", errors.New("proxmox setup: invalid network inventory")
	}
	bridge := ""
	for _, n := range networks {
		if n.Type == "bridge" && n.Active == 1 && (bridge == "" || n.Interface == DefaultBridge) {
			bridge = n.Interface
		}
	}
	if bridge == "" {
		return "", errors.New("proxmox setup: create an active network bridge on this node, then retry")
	}
	return bridge, nil
}

// The setup writes two things into a template: its own marker, and the release
// the image was prepared from. Both go in the description, which is the one
// field of a powered-off guest the API hands back -- the same reason the marks
// for the machines themselves live there.
//
// The release is recorded because a rented machine installs nothing of its own:
// its agent is the one the template carried when it was cloned, so the template
// is the only thing that can bring a fleet's machines to the release its
// controller runs. Without the record a later setup command cannot tell a
// template prepared this morning from one prepared last quarter.
const (
	// templateMarkerPrefix is the first line of the description of a template
	// this setup made. It is the text it has always been: a template built by
	// an older release has to stay recognisable as ours, or the fleet whose
	// template most needs preparing again would be the one refused as a
	// stranger's guest.
	templateMarkerPrefix = "Zoomies template setup "
	// templateAgentPrefix begins the second line, which names the release.
	templateAgentPrefix = "agent: "
)

// templateDiskSize is the size a template's disk is given. Proxmox refuses a
// resize to the size a disk already has, so a run that finds one at this size
// has to be able to finish without it.
const templateDiskSize = "32G"

func templateMarker(key string) string { return templateMarkerPrefix + key }

// templateDescription is what the setup writes into a template it prepares.
func templateDescription(key, build string) string {
	description := templateMarker(key)
	if strings.TrimSpace(build) != "" {
		description += "\n" + templateAgentPrefix + build
	}
	return description
}

// templateBuild reads a description back: the release the template records it
// was prepared from, and whether the description is this setup's at all. The
// second answer is what keeps a guest somebody else made under the template's
// name out of the way, and it is the only evidence there is: the template is
// powered off, so nothing can be read from inside it.
func templateBuild(description, key string) (build string, ours bool) {
	lines := strings.Split(strings.ReplaceAll(description, "\r\n", "\n"), "\n")
	if strings.TrimSpace(lines[0]) != templateMarker(key) {
		return "", false
	}
	for _, line := range lines[1:] {
		if rest, ok := strings.CutPrefix(strings.TrimSpace(line), templateAgentPrefix); ok {
			return strings.TrimSpace(rest), true
		}
	}
	return "", true
}

// templateBehind says whether a template prepared from recorded has to be
// prepared again for a fleet running build.
//
// A template that records no release is behind, because one that does not say
// what it carries cannot be the release this controller runs. One already
// carrying this build, or a later one, is left alone: this fleet never takes an
// agent backwards, and the image every agent here is cloned from is no
// different.
func templateBehind(recorded, build string) bool {
	switch {
	case strings.TrimSpace(build) == "":
		// Nothing to prepare it from: a build that cannot name itself cannot
		// be written into an image.
		return false
	case strings.TrimSpace(recorded) == "":
		return true
	}
	switch version.CompareBuilds(recorded, build) {
	case version.SkewBehind, version.SkewDiffers:
		return true
	}
	return false
}

// guest is one row of the cluster's guest inventory.
type guest struct {
	VMID     int    `json:"vmid"`
	Name     string `json:"name"`
	Node     string `json:"node"`
	Type     string `json:"type"`
	Template int    `json:"template"`
}

// preparation is one run of the setup: what it needs to make the runner image,
// finish one an earlier run was interrupted in the middle of, or prepare a
// template again. It is a value rather than a list of arguments because the
// three paths share the image preparation and the stamp.
type preparation struct {
	ctx    context.Context
	h      Host
	run    func(name string, args ...string) ([]byte, error)
	dir    string
	binary string
	node   string
	key    string
	build  string
	out    io.Writer
}

// EnsureTemplate prepares the image locally, before reserving a VMID. An
// interrupted import is resumed only when the VM carries this setup's marker;
// a guest with the same name is never enough evidence to change it.
//
// build is the release the image carries into the guest, and the release the
// template records it was prepared from. A template this setup made that
// records an earlier release is prepared again in place, at the same VMID and
// the same name: a machine is cloned from that image and installs nothing of
// its own, so preparing the template again is how the machines built next come
// to run the release this controller runs. A guest with the template's name
// that this setup did not make is used exactly as it is found.
//
// What it found and what it did are written to out, one line per template, for
// the person who ran the command: a preparation that takes minutes with nothing
// to say reads as a hang.
func EnsureTemplate(ctx context.Context, h Host, dir, binary, node, key, build string, out io.Writer) ([]Template, error) {
	p := &preparation{ctx: ctx, h: h, dir: dir, binary: binary, node: node, key: key, build: build, out: out}
	p.run = func(name string, args ...string) ([]byte, error) {
		raw, err := h.Run(ctx, name, args...)
		if err != nil {
			return nil, fmt.Errorf("proxmox setup: %s %s failed; correct the host configuration and rerun setup", name, args[0])
		}
		return raw, nil
	}
	raw, err := p.run("pvesh", "get", "/cluster/resources", "--type", "vm", "--output-format", "json")
	if err != nil {
		return nil, err
	}
	var guests []guest
	if json.Unmarshal(raw, &guests) != nil {
		return nil, errors.New("proxmox setup: invalid VM inventory")
	}
	var templates []Template
	vmid := 0
	preferred := 0
	for _, vm := range guests {
		if vm.Type != "qemu" {
			continue
		}
		if vm.Template == 1 {
			templates = append(templates, Template{VMID: vm.VMID, Name: vm.Name, Node: vm.Node})
		}
		if vm.Name != TemplateName {
			continue
		}
		if vm.Template == 1 {
			preferred = vm.VMID
			continue
		}
		if vmid != 0 || vm.Node != node {
			return nil, errors.New("proxmox setup: a VM named zoomies-template already exists; finish or rename it before retrying")
		}
		vmid = vm.VMID
	}
	if preferred != 0 {
		return p.keepOrPrepareAgain(templates, preferred)
	}
	return p.buildOrResume(templates, vmid)
}

// keepOrPrepareAgain is the path taken when a template called zoomies-template
// already exists, which is the one the wizard picks and the one every machine
// this provider builds is cloned from.
//
// A template this setup made that records an earlier release than the one
// running here is prepared again. The old disk is left unattached rather than
// deleted, because it is the copy the machines running now were cloned from and
// the only thing that would put the template back if the new image turned out
// to be wrong.
func (p *preparation) keepOrPrepareAgain(templates []Template, vmid int) ([]Template, error) {
	tpl := Template{VMID: vmid, Name: TemplateName}
	for _, t := range templates {
		if t.VMID == vmid {
			tpl = t
		}
	}
	config, err := p.guestConfig(tpl.Node, vmid)
	if err != nil {
		return nil, err
	}
	recorded, ours := templateBuild(text(config["description"]), p.key)
	if !ours {
		fmt.Fprintf(p.out, "Template %d: %s (node %s) was not prepared by Zoomies, so it is left exactly as it is found; prepare it as you prepared it when its release moves on.\n",
			vmid, tpl.Name, tpl.Node)
		return p.replace(templates, tpl), nil
	}
	if !templateBehind(recorded, p.build) {
		tpl.Agent = recorded
		fmt.Fprintf(p.out, "Template %d: %s (node %s) already carries %s.\n", vmid, tpl.Name, tpl.Node, namedBuild(recorded))
		return p.replace(templates, tpl), nil
	}
	if tpl.Node != p.node {
		// The image is imported into a guest from the node that guest is on,
		// and it is prepared beside this setup's own state on the node this
		// command runs on.
		fmt.Fprintf(p.out, "Template %d: %s is on %s and carries %s; run the setup command on that node to prepare it again from %s.\n",
			vmid, tpl.Name, tpl.Node, namedBuild(recorded), p.build)
		tpl.Agent = recorded
		return p.replace(templates, tpl), nil
	}
	if err := p.prepareAgain(tpl, config); err != nil {
		return nil, err
	}
	tpl.Agent = p.build
	fmt.Fprintf(p.out, "Template %d: %s was prepared from %s and has been prepared again from %s.\n",
		vmid, tpl.Name, namedBuild(recorded), p.build)
	return p.replace(templates, tpl), nil
}

// replace puts a template back into the inventory with what this run learned
// about it, and orders the list the way every other answer is ordered: the one
// the wizard should offer first.
func (p *preparation) replace(templates []Template, tpl Template) []Template {
	for i := range templates {
		if templates[i].VMID == tpl.VMID {
			templates[i] = tpl
		}
	}
	return templatesWithPreferred(templates, tpl.VMID)
}

// buildOrResume makes the runner template, or finishes making one an earlier
// run was interrupted in the middle of.
//
// The marker is read before anything is changed. A guest carrying the
// template's name with somebody else's description is not ours, and a
// half-imported disk is only ours when the description says so.
func (p *preparation) buildOrResume(templates []Template, vmid int) ([]Template, error) {
	description := templateDescription(p.key, p.build)
	config := map[string]any{}
	if vmid != 0 {
		var err error
		config, err = p.guestConfig(p.node, vmid)
		if err != nil {
			return nil, err
		}
		if _, ours := templateBuild(text(config["description"]), p.key); !ours {
			return nil, errors.New("proxmox setup: the existing zoomies-template VM was not created by this setup; finish or rename it before retrying")
		}
	}
	if config["scsi0"] == nil {
		raw, err := p.run("pvesh", "get", "/nodes/"+p.node+"/storage", "--output-format", "json")
		if err != nil {
			return nil, err
		}
		var storages []struct {
			Storage string `json:"storage"`
			Content string `json:"content"`
			Active  int    `json:"active"`
			Avail   int64  `json:"avail"`
		}
		if json.Unmarshal(raw, &storages) != nil {
			return nil, errors.New("proxmox setup: invalid storage inventory")
		}
		storage := ""
		var available int64
		for _, s := range storages {
			if s.Active != 1 || !strings.Contains(","+s.Content+",", ",images,") || s.Avail < 33<<30 {
				continue
			}
			if storage == "" || s.Avail > available {
				storage, available = s.Storage, s.Avail
			}
		}
		if storage == "" {
			return nil, errors.New("proxmox setup: enable storage for disk images with at least 33 GiB free on this node, then retry")
		}
		bridge, err := ActiveBridge(p.ctx, p.h, p.node)
		if err != nil {
			return nil, err
		}
		image, err := p.prepareImage()
		if err != nil {
			return nil, err
		}
		if vmid == 0 {
			// The default runner block is 9000-9099. Keep the template outside it.
			raw, err := p.run("pvesh", "get", "/cluster/nextid", "--vmid", "9100")
			if err != nil {
				return nil, err
			}
			vmid, err = strconv.Atoi(strings.TrimSpace(string(raw)))
			if err != nil || vmid < 9100 || vmid > 999999999 {
				return nil, errors.New("proxmox setup: Proxmox did not return an available template VMID")
			}
			if _, err := p.run("qm", "create", strconv.Itoa(vmid), "--name", TemplateName, "--description", description,
				"--memory", "4096", "--cores", "2", "--cpu", "host", "--ostype", "l26",
				"--net0", "virtio,bridge="+bridge, "--scsihw", "virtio-scsi-pci", "--agent", "enabled=1", "--serial0", "socket", "--vga", "serial0"); err != nil {
				return nil, err
			}
			config["description"] = description
		}
		id := strconv.Itoa(vmid)
		if _, err := p.run("qm", "set", id, "--scsi0", storage+":0,import-from="+image, "--boot", "order=scsi0"); err != nil {
			return nil, err
		}
		config["scsi0"] = storage + ":disk"
	}
	id := strconv.Itoa(vmid)
	if config["ide2"] == nil {
		storage, _, ok := strings.Cut(text(config["scsi0"]), ":")
		if !ok || storage == "" {
			return nil, errors.New("proxmox setup: the interrupted template has an invalid disk; inspect it before retrying")
		}
		if _, err := p.run("qm", "set", id, "--ide2", storage+":cloudinit", "--ciuser", "ubuntu", "--ipconfig0", "ip=dhcp"); err != nil {
			return nil, err
		}
	}
	// Proxmox refuses an equal-size resize. A retry after a successful resize
	// must still be able to finish the conversion.
	disk := text(config["scsi0"])
	if !strings.Contains(","+disk+",", ",size="+templateDiskSize+",") {
		if _, err := p.run("qm", "resize", id, "scsi0", templateDiskSize); err != nil {
			return nil, err
		}
	}
	// A template an earlier release made records no release, and this is where
	// it learns which one it carries: after the disk is in place, so that a
	// failure before this point leaves a template that is still honestly
	// described as unknown rather than wrongly described as current.
	if text(config["description"]) != description {
		if _, err := p.run("qm", "set", id, "--description", description); err != nil {
			return nil, err
		}
	}
	if _, err := p.run("qm", "template", id); err != nil {
		return nil, err
	}
	// The imported disk is now owned by Proxmox; keep no second image copy on
	// the host once conversion has succeeded.
	_, _ = p.h.Run(p.ctx, "rm", "-f", "--", filepath.Join(p.dir, "template.img"))
	tpl := Template{VMID: vmid, Name: TemplateName, Node: p.node, Agent: p.build}
	return p.replace(append(templates, tpl), tpl), nil
}

// prepareImage renders the unit the image ships and runs the image preparation
// on the node, which is the minutes-long half of setup. It returns where the
// prepared image is.
func (p *preparation) prepareImage() (string, error) {
	unit, err := installer.RenderSystemdUnit(installer.ServiceSpec{
		Unit: installer.UnitAgent, ExecPath: "/usr/local/bin/zoomies", ConfigFile: "/etc/zoomies/zoomies.yaml",
		User: "zoomies", Group: "zoomies", StateDir: "/var/lib/zoomies", ConfigDir: "/etc/zoomies",
		EnvFile: installer.MachineEnvPath, WantsDocker: true, RuntimeName: "docker", SupplementaryGroups: []string{"docker"},
	})
	if err != nil {
		return "", err
	}
	unitPath := filepath.Join(p.dir, "zoomies-agent.service")
	if err := p.h.WriteFile(unitPath, []byte(unit), 0600); err != nil {
		return "", err
	}
	if _, err := p.run("bash", "-c", prepareTemplateImage, TemplateName, p.dir, p.binary, unitPath); err != nil {
		return "", errors.New("proxmox setup: runner image preparation failed; check outbound access to cloud-images.ubuntu.com and Ubuntu package mirrors, and that libguestfs-tools can be installed, then retry")
	}
	return filepath.Join(p.dir, "template.img"), nil
}

// prepareAgain puts a freshly prepared image in place of the disk a template
// already has, at the same VMID and the same name, so a provider row that
// points at it needs no change and every machine cloned from it from now on
// carries the release this setup is running.
//
// Nothing is touched but the template, and it is not running: a clone has its
// own copy of the disk, so the machines built from the old image keep working
// on it for as long as they live.
func (p *preparation) prepareAgain(tpl Template, config map[string]any) error {
	disk := text(config["scsi0"])
	storage, rest, ok := strings.Cut(disk, ":")
	if !ok || storage == "" || rest == "" {
		return fmt.Errorf("proxmox setup: template %d has no operating-system disk this setup can replace; prepare it as you prepared it, and delete this command's line for it", tpl.VMID)
	}
	old, settings, _ := strings.Cut(rest, ",")
	size := ""
	if value, ok := strings.CutPrefix(strings.TrimSpace(settings), "size="); ok {
		size = strings.TrimSpace(value)
	}
	if size == "" {
		size = templateDiskSize
	}
	image, err := p.prepareImage()
	if err != nil {
		return err
	}
	id := strconv.Itoa(tpl.VMID)
	if _, err := p.run("qm", "set", id, "--scsi0", storage+":0,import-from="+image); err != nil {
		return fmt.Errorf("proxmox setup: the prepared image could not be put into template %d, which is unchanged; the machines already cloned from it are unaffected: %w", tpl.VMID, err)
	}
	if _, err := p.run("qm", "resize", id, "scsi0", size); err != nil {
		return fmt.Errorf("proxmox setup: template %d holds the prepared image but could not be given its disk size back; set that by hand on the node and prepare the template again: %w", tpl.VMID, err)
	}
	if _, err := p.run("qm", "set", id, "--description", templateDescription(p.key, p.build)); err != nil {
		return err
	}
	_, _ = p.h.Run(p.ctx, "rm", "-f", "--", image)
	p.reportLeftoverDisk(tpl, storage, storage+":"+old)
	return nil
}

// reportLeftoverDisk names the disk the import left behind, which Proxmox moves
// to the first free unused slot. It is named rather than removed: it is the
// copy the machines running now were cloned from, and removing it is the
// operator's to do once the machines built from the new image have proved it.
func (p *preparation) reportLeftoverDisk(tpl Template, storage, volume string) {
	after, err := p.guestConfig(tpl.Node, tpl.VMID)
	if err != nil {
		return
	}
	var slots []string
	for slot, value := range after {
		if !strings.HasPrefix(slot, "unused") {
			continue
		}
		if text(value) == volume {
			slots = append(slots, slot)
		}
	}
	sort.Strings(slots)
	for _, slot := range slots {
		fmt.Fprintf(p.out, "  the disk the machines running now were cloned from is left on %s as %s; remove it with `qm set %d --delete %s` once those machines are gone.\n",
			storage, volume, tpl.VMID, slot)
	}
}

// guestConfig reads one guest's recorded configuration.
func (p *preparation) guestConfig(node string, vmid int) (map[string]any, error) {
	raw, err := p.run("pvesh", "get", fmt.Sprintf("/nodes/%s/qemu/%d/config", node, vmid), "--output-format", "json")
	if err != nil {
		return nil, err
	}
	config := map[string]any{}
	if json.Unmarshal(raw, &config) != nil {
		return nil, errors.New("proxmox setup: invalid guest configuration")
	}
	return config, nil
}

// text is one field of a guest's configuration as Proxmox wrote it: a string,
// or nothing at all, which is how a field a guest does not have arrives.
func text(value any) string {
	s, _ := value.(string)
	return strings.TrimSpace(s)
}

// namedBuild is a release as a sentence names it, with something to say when
// there is none.
func namedBuild(build string) string {
	if strings.TrimSpace(build) == "" {
		return "a release it does not record"
	}
	return build
}

func templatesWithPreferred(templates []Template, id int) []Template {
	// The callback acknowledges byte-identical retries after a lost response.
	// Cluster inventory order must not change its payload between runs.
	sort.Slice(templates, func(i, j int) bool {
		if templates[i].VMID == id {
			return templates[j].VMID != id
		}
		if templates[j].VMID == id {
			return false
		}
		return templates[i].VMID < templates[j].VMID
	})
	return templates
}

// Only the guest image receives Docker and the runner service. The host's
// runner installation and credentials never enter the image.
const prepareTemplateImage = `set -eu
dir=$1
binary=$2
unit=$3
[ "$(uname -m)" = x86_64 ] || { echo 'Automatic templates need an x86_64 Proxmox host'; exit 1; }
if ! command -v virt-customize >/dev/null; then
  apt-get update
  DEBIAN_FRONTEND=noninteractive apt-get install -y libguestfs-tools
fi
tmp=$(mktemp -d "$dir/image.XXXXXX")
trap 'rm -rf "$tmp"' EXIT
base=https://cloud-images.ubuntu.com/noble/current
asset=noble-server-cloudimg-amd64.img
curl --proto '=https' --proto-redir '=https' -fsSL "$base/$asset" -o "$tmp/$asset"
curl --proto '=https' --proto-redir '=https' -fsSL "$base/SHA256SUMS" -o "$tmp/SHA256SUMS"
(cd "$tmp"; awk -v asset="$asset" '$2 == asset || $2 == "*" asset { print; found=1 } END { if (!found) exit 1 }' SHA256SUMS > selected; sha256sum -c selected)
export LIBGUESTFS_BACKEND=direct
qemu-img create -f qcow2 "$tmp/prepared.img" 8G
virt-resize --expand /dev/sda1 "$tmp/$asset" "$tmp/prepared.img"
# virt-resize renumbers the partitions in disk order (the image's 1, 14, 15, 16
# become 1 to 4), and the boot loader it copies still names the old numbers, so
# a machine cloned from this image stops in grub rescue with "no such
# partition". The boot loader is written again against the layout it now has.
#
# The machine id is left empty, not removed: an empty file is how systemd knows
# to make a new one on first boot, and without any file its DHCP client fails
# ("Failed to configure DHCPv4 client: No such file or directory"), so a clone
# boots with no address and its agent can never reach the controller.
virt-customize -a "$tmp/prepared.img" --install docker.io,qemu-guest-agent \
  --upload "$binary:/usr/local/bin/zoomies" --chmod 0755:/usr/local/bin/zoomies \
  --upload "$unit:/etc/systemd/system/zoomies-agent.service" --chmod 0644:/etc/systemd/system/zoomies-agent.service \
  --run-command 'set -e; useradd --system --user-group --home-dir /var/lib/zoomies --shell /usr/sbin/nologin zoomies; usermod -aG docker zoomies' \
  --run-command 'set -e; mkdir -p /etc/zoomies /var/lib/zoomies/work; printf "agent:\n  embedded: false\n  work_dir: /var/lib/zoomies/work\n" > /etc/zoomies/zoomies.yaml; chown -R zoomies:zoomies /etc/zoomies /var/lib/zoomies' \
  --run-command 'set -e; systemctl enable docker.service qemu-guest-agent.service; systemctl disable zoomies-agent.service' \
  --run-command 'set -e; grub-install --target=i386-pc /dev/sda; update-grub' \
  --run-command 'set -e; cloud-init clean --logs --machine-id; rm -f /etc/ssh/ssh_host_*; rm -f /var/lib/dbus/machine-id; : > /etc/machine-id'
mv "$tmp/prepared.img" "$dir/template.img"
`
