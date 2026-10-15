package proxmoxsetup

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"
)

// templateBuild is the release every test prepares a template from, so that a
// test asserting "it records what it carries" reads as the setup's own answer.
const testBuild = "v1.4.0"

// prepareTemplate is EnsureTemplate with a test's own build and a throwaway
// output, for the tests that ask about the guests rather than about the words.
func prepareTemplate(t *testing.T, h Host, inventory, config, build string) ([]Template, string, error) {
	t.Helper()
	var out strings.Builder
	templates, err := EnsureTemplate(context.Background(), h, "/provider", "/provider/zoomies", "pve", "abc", build, &out)
	return templates, out.String(), err
}

func templateHost(t *testing.T, inventory, config string) (Host, *[]string) {
	t.Helper()
	var commands []string
	h := Host{
		WriteFile: func(path string, data []byte, mode os.FileMode) error {
			if !strings.HasSuffix(path, "/zoomies-agent.service") || mode != 0600 {
				t.Fatalf("unexpected host write: %s %v", path, mode)
			}
			for _, want := range []string{"EnvironmentFile=-/etc/zoomies/zoomies.env", "SupplementaryGroups=docker", "zoomies agent"} {
				if !strings.Contains(string(data), want) {
					t.Errorf("runner unit missing %q", want)
				}
			}
			return nil
		},
		Run: func(_ context.Context, name string, args ...string) ([]byte, error) {
			line := name + " " + strings.Join(args, " ")
			commands = append(commands, line)
			switch {
			case strings.Contains(line, "/cluster/resources"):
				return []byte(inventory), nil
			case strings.Contains(line, "/config"):
				return []byte(config), nil
			case strings.Contains(line, "/storage"):
				return []byte(`[{"storage":"backup","active":1,"content":"backup","avail":999999999999},{"storage":"local","active":1,"content":"images,iso","avail":50000000000}]`), nil
			case strings.Contains(line, "/network"):
				return []byte(`[{"iface":"vmbr1","type":"bridge","active":1},{"iface":"vmbr0","type":"bridge","active":1}]`), nil
			case strings.Contains(line, "/cluster/nextid"):
				return []byte("9101\n"), nil
			}
			return nil, nil
		},
	}
	return h, &commands
}

func TestSetupCreatesTheRunnerTemplateAlongsideUnrelatedTemplates(t *testing.T) {
	h, commands := templateHost(t, `[{"vmid":200,"name":"other-template","node":"pve","type":"qemu","template":1}]`, "")
	templates, _, err := prepareTemplate(t, h, `[{"vmid":200,"name":"other-template","node":"pve","type":"qemu","template":1}]`, "", testBuild)
	if err != nil {
		t.Fatal(err)
	}
	if len(templates) != 2 || templates[0].VMID != 9101 || templates[0].Name != TemplateName || templates[0].Node != "pve" || templates[0].Agent != testBuild {
		t.Fatalf("wrong template inventory: %+v", templates)
	}
	joined := strings.Join(*commands, "\n")
	for _, want := range []string{"/cluster/nextid --vmid 9100", "qm create 9101", "--net0 virtio,bridge=vmbr0", "--scsi0 local:0,import-from=/provider/template.img", "--ide2 local:cloudinit", "qm resize 9101 scsi0 32G", "qm template 9101"} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing %q", want)
		}
	}
	// The image records the release it carries, so that a setup command run
	// after the controller has moved on can tell whether the machines cloned
	// from this template are current.
	if !strings.Contains(joined, "agent: "+testBuild) {
		t.Errorf("the template does not record the release it was prepared from: %v", *commands)
	}
	if strings.Contains(joined, "backup:0") || strings.Contains(joined, "qm create 200") {
		t.Fatal("used backup storage or changed an unrelated template")
	}
}

func TestSetupLeavesATemplateItDidNotMakeExactlyAsItIsFound(t *testing.T) {
	// The runbook's recipe has an operator build this image by hand and name it
	// zoomies-template, and the wizard offers it. It is not ours to change: it
	// carries no marker, so nothing here can know what is in it.
	h, commands := templateHost(t, `[{"vmid":200,"name":"other-template","node":"pve","type":"qemu","template":1},{"vmid":9105,"name":"zoomies-template","node":"pve-2","type":"qemu","template":1}]`,
		`{"description":"ubuntu 24.04 runner image","scsi0":"local-lvm:vm-9105-disk-0,size=32G"}`)
	templates, out, err := prepareTemplate(t, h, `[{"vmid":200,"name":"other-template","node":"pve","type":"qemu","template":1},{"vmid":9105,"name":"zoomies-template","node":"pve-2","type":"qemu","template":1}]`,
		`{"description":"ubuntu 24.04 runner image","scsi0":"local-lvm:vm-9105-disk-0,size=32G"}`, testBuild)
	if err != nil || len(templates) != 2 || templates[0].VMID != 9105 || templates[0].Node != "pve-2" || templates[0].Agent != "" {
		t.Fatalf("did not reuse the template: %+v %v", templates, err)
	}
	joined := strings.Join(*commands, "\n")
	if strings.Contains(joined, "qm set") || strings.Contains(joined, "bash ") {
		t.Fatalf("changed a guest this setup did not make: %v", *commands)
	}
	if !strings.Contains(out, "not prepared by Zoomies") {
		t.Errorf("the operator is not told the template is used as found: %q", out)
	}
}

func TestSetupPreparesATemplateItMadeAgainWhenItCarriesAnOlderRelease(t *testing.T) {
	inventory := `[{"vmid":200,"name":"other-template","node":"pve","type":"qemu","template":1},{"vmid":9100,"name":"zoomies-template","node":"pve","type":"qemu","template":1}]`
	before := `{"description":"Zoomies template setup abc\nagent: v1.3.1","scsi0":"local-lvm:vm-9100-disk-0,size=64G","ide2":"local-lvm:vm-9100-cloudinit,media=cdrom"}`
	after := `{"description":"Zoomies template setup abc\nagent: v1.4.0","scsi0":"local-lvm:vm-9100-disk-1,size=32G","unused0":"local-lvm:vm-9100-disk-0","ide2":"local-lvm:vm-9100-cloudinit,media=cdrom"}`
	h, commands, out := refreshHost(t, inventory, before, after)
	templates, err := EnsureTemplate(context.Background(), h, "/provider", "/provider/zoomies", "pve", "abc", testBuild, out)
	if err != nil {
		t.Fatal(err)
	}
	if len(templates) != 2 || templates[0].VMID != 9100 || templates[0].Agent != testBuild {
		t.Fatalf("the prepared template is not what the callback is told: %+v", templates)
	}
	joined := strings.Join(*commands, "\n")
	for _, want := range []string{
		// The image is made again from the release this setup runs.
		"bash -c",
		// It goes into the same guest, at the same VMID and name, so the
		// provider row that points at it needs no change.
		"qm set 9100 --scsi0 local-lvm:0,import-from=/provider/template.img",
		// The disk the operator had sized by hand is kept at its size, not
		// reset to the stock one.
		"qm resize 9100 scsi0 64G",
		"qm set 9100 --description",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing %q in:\n%s", want, joined)
		}
	}
	if strings.Contains(joined, "qm create") {
		t.Fatalf("made a second guest rather than preparing the template again: %v", *commands)
	}
	if strings.Contains(joined, "--delete") {
		t.Fatalf("deleted a disk without the operator asking: %v", *commands)
	}
	// The old disk is named, with the command that removes it, because leaving
	// it is a decision the operator has to know about.
	if !strings.Contains(out.String(), "prepared again from "+testBuild) || !strings.Contains(out.String(), "--delete unused0") {
		t.Errorf("the operator is not told what happened:\n%s", out.String())
	}
}

func TestSetupLeavesATemplateOnAnotherNodeAndSaysWhereToPrepareIt(t *testing.T) {
	inventory := `[{"vmid":9105,"name":"zoomies-template","node":"pve-2","type":"qemu","template":1}]`
	config := `{"description":"Zoomies template setup abc\nagent: v1.3.1","scsi0":"local-lvm:vm-9105-disk-0,size=32G"}`
	h, commands, out := refreshHost(t, inventory, config, config)
	templates, err := EnsureTemplate(context.Background(), h, "/provider", "/provider/zoomies", "pve", "abc", testBuild, out)
	if err != nil || len(templates) != 1 || templates[0].VMID != 9105 {
		t.Fatalf("EnsureTemplate = %+v, %v", templates, err)
	}
	// The image is imported into a guest from the node that guest is on, and it
	// is prepared beside this setup's own state: a command run on the wrong node
	// must not leave a guest holding half an image.
	if strings.Contains(strings.Join(*commands, "\n"), "qm set") || strings.Contains(strings.Join(*commands, "\n"), "bash ") {
		t.Fatalf("prepared a template from the wrong node: %v", *commands)
	}
	if !strings.Contains(out.String(), "run the setup command on that node") {
		t.Errorf("the operator is not told where to run it: %q", out.String())
	}
}

func TestSetupLeavesATemplateThatAlreadyCarriesThisReleaseAlone(t *testing.T) {
	inventory := `[{"vmid":9100,"name":"zoomies-template","node":"pve","type":"qemu","template":1}]`
	config := `{"description":"Zoomies template setup abc\nagent: v1.4.0","scsi0":"local-lvm:vm-9100-disk-0,size=32G"}`
	h, commands, out := refreshHost(t, inventory, config, config)
	if _, err := EnsureTemplate(context.Background(), h, "/provider", "/provider/zoomies", "pve", "abc", testBuild, out); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.Join(*commands, "\n"), "bash ") {
		t.Fatalf("rebuilt an image that was already current: %v", *commands)
	}
	if !strings.Contains(out.String(), "already carries "+testBuild) {
		t.Errorf("the operator is not told it was left alone: %q", out.String())
	}
}

// refreshHost is a node with one template, answering the second read of its
// configuration with what a replaced disk looks like: a new volume on scsi0 and
// the old one moved to the first unused slot.
func refreshHost(t *testing.T, inventory, before, after string) (Host, *[]string, *strings.Builder) {
	t.Helper()
	var commands []string
	reads := 0
	out := &strings.Builder{}
	h := Host{
		WriteFile: func(string, []byte, os.FileMode) error { return nil },
		Run: func(_ context.Context, name string, args ...string) ([]byte, error) {
			line := name + " " + strings.Join(args, " ")
			commands = append(commands, line)
			switch {
			case strings.Contains(line, "/cluster/resources"):
				return []byte(inventory), nil
			case strings.Contains(line, "/config"):
				reads++
				if reads > 1 {
					return []byte(after), nil
				}
				return []byte(before), nil
			}
			return nil, nil
		},
	}
	return h, &commands, out
}

func TestTemplateBuildOutlivesTheSetupThatWroteIt(t *testing.T) {
	// A template built by an older release carries the marker and nothing else,
	// and has to stay recognisable as ours: it is exactly the fleet whose
	// machines are behind that a stranger's-guest refusal would strand.
	if build, ours := templateBuild(templateMarker("abc"), "abc"); !ours || build != "" {
		t.Fatalf("marker only = %q, %v; want ours with no release", build, ours)
	}
	if build, ours := templateBuild(templateDescription("abc", testBuild), "abc"); !ours || build != testBuild {
		t.Fatalf("round trip = %q, %v", build, ours)
	}
	if build, ours := templateBuild(templateDescription("abc", testBuild), "other"); ours || build != "" {
		t.Fatalf("another setup's template = %q, %v; want not ours", build, ours)
	}
	if _, ours := templateBuild("ubuntu 24.04 runner image", "abc"); ours {
		t.Fatal("a guest with no marker was read as ours")
	}
	if _, ours := templateBuild("", "abc"); ours {
		t.Fatal("a guest with no description was read as ours")
	}
	if build, _ := templateBuild("Zoomies template setup abc\r\nagent: v1.4.0\r\n", "abc"); build != testBuild {
		t.Fatalf("a description with carriage returns = %q", build)
	}
}

func TestTemplateBehindNeverTakesAFleetBackwards(t *testing.T) {
	for name, tc := range map[string]struct {
		recorded, build string
		want            bool
	}{
		"the release this setup runs":        {testBuild, testBuild, false},
		"an earlier release":                 {"v1.3.1", testBuild, true},
		"a later release":                    {"v1.5.0", testBuild, false},
		"a pre-release of this one":          {"v1.4.0-rc1", testBuild, true},
		"the same release written two ways":  {"1.4", testBuild, false},
		"nothing recorded":                   {"", testBuild, true},
		"a build that cannot name itself":    {"", "", false},
		"a release against nothing recorded": {testBuild, "", false},
		"a build from main":                  {"main-sha-abc1234", "v1.4.0", true},
	} {
		t.Run(name, func(t *testing.T) {
			if got := templateBehind(tc.recorded, tc.build); got != tc.want {
				t.Errorf("templateBehind(%q, %q) = %v, want %v", tc.recorded, tc.build, got, tc.want)
			}
		})
	}
}

func TestTemplateCallbackOrderSurvivesAnInventoryReorder(t *testing.T) {
	first := templatesWithPreferred([]Template{{VMID: 200}, {VMID: 300}, {VMID: 9100, Name: TemplateName}}, 9100)
	second := templatesWithPreferred([]Template{{VMID: 300}, {VMID: 9100, Name: TemplateName}, {VMID: 200}}, 9100)
	for i := range first {
		if first[i] != second[i] {
			t.Fatalf("retry changed callback order: %v %v", first, second)
		}
	}
}

func TestSetupRefusesAnUnrelatedGuestWithTheTemplateName(t *testing.T) {
	inventory := `[{"vmid":101,"name":"zoomies-template","node":"pve","type":"qemu"}]`
	config := `{"description":"somebody else's VM","scsi0":"local:disk"}`
	h, commands := templateHost(t, inventory, config)
	if _, _, err := prepareTemplate(t, h, inventory, config, testBuild); err == nil {
		t.Fatal("accepted an unrelated guest")
	}
	if len(*commands) != 2 {
		t.Fatalf("modified an unrelated guest: %v", *commands)
	}
}

func TestTemplatePreparationFailureNeverCreatesAVMOrReportsReady(t *testing.T) {
	for _, stage := range []string{"/cluster/resources", "bash -c", "qm template"} {
		t.Run(stage, func(t *testing.T) {
			h, commands := templateHost(t, `[]`, "")
			run := h.Run
			h.Run = func(ctx context.Context, name string, args ...string) ([]byte, error) {
				if strings.Contains(name+" "+strings.Join(args, " "), stage) {
					return nil, errors.New("failed")
				}
				return run(ctx, name, args...)
			}
			templates, _, err := prepareTemplate(t, h, `[]`, "", testBuild)
			if err == nil || templates != nil {
				t.Fatalf("failed template reported ready: %v %v", templates, err)
			}
			if stage != "qm template" && strings.Contains(strings.Join(*commands, "\n"), "qm create") {
				t.Fatal("created a VM before the image was ready")
			}
		})
	}
}

func TestImagePreparationScriptIsValidAndLeavesAnUnenrolledAgent(t *testing.T) {
	cmd := exec.Command("bash", "-n")
	cmd.Stdin = strings.NewReader(prepareTemplateImage)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("invalid image script: %v %s", err, out)
	}
	for _, want := range []string{"sha256sum -c", "--install docker.io,qemu-guest-agent", "systemctl disable zoomies-agent.service", "cloud-init clean --logs --machine-id"} {
		if !strings.Contains(prepareTemplateImage, want) {
			t.Errorf("missing %q", want)
		}
	}
}

// virt-resize renumbers a cloud image's partitions, and the boot loader it
// copies still names the old ones: a machine cloned from the result sits in
// "grub rescue" with "no such partition", never boots and never answers its
// guest agent. The boot loader has to be written again after the resize.
func TestImagePreparationWritesTheBootLoaderAgainAfterTheResize(t *testing.T) {
	resize := strings.Index(prepareTemplateImage, "virt-resize")
	grub := strings.Index(prepareTemplateImage, "grub-install --target=i386-pc /dev/sda")
	if resize < 0 || grub < 0 || grub < resize {
		t.Fatalf("the boot loader is not written again after the resize (resize at %d, grub-install at %d)", resize, grub)
	}
}

func bridgeHost(inventory string) Host {
	return Host{Run: func(_ context.Context, name string, args ...string) ([]byte, error) {
		if name != "pvesh" || !strings.Contains(strings.Join(args, " "), "/nodes/pve/network") {
			return nil, errors.New("unexpected command")
		}
		return []byte(inventory), nil
	}}
}

// A node whose bridge is not vmbr0 is the case that used to be saved with a
// bridge that does not exist, so the choice has to follow the node.
func TestActiveBridgePrefersVmbr0ButFollowsTheNode(t *testing.T) {
	for name, tc := range map[string]struct{ inventory, want string }{
		"vmbr0 among others": {`[{"iface":"vmbr1","type":"bridge","active":1},{"iface":"vmbr0","type":"bridge","active":1}]`, "vmbr0"},
		"no vmbr0":           {`[{"iface":"eno1","type":"eth","active":1},{"iface":"vmbr7","type":"bridge","active":1}]`, "vmbr7"},
		"vmbr0 is down":      {`[{"iface":"vmbr0","type":"bridge","active":0},{"iface":"vmbr1","type":"bridge","active":1}]`, "vmbr1"},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := ActiveBridge(context.Background(), bridgeHost(tc.inventory), "pve")
			if err != nil || got != tc.want {
				t.Fatalf("ActiveBridge = %q, %v; want %q", got, err, tc.want)
			}
		})
	}
	if _, err := ActiveBridge(context.Background(), bridgeHost(`[{"iface":"eno1","type":"eth","active":1}]`), "pve"); err == nil {
		t.Fatal("a node with no bridge was given one")
	}
}

func TestAConnectionRefusesAnUnsafeBridgeName(t *testing.T) {
	c := Connection{Name: "proxmox-pve", Endpoint: "https://pve:8006", Credential: "u@pve!t=secret", Bridge: "vmbr0; reboot"}
	if err := c.Validate(); err == nil || !strings.Contains(err.Error(), "bridge") {
		t.Fatalf("Validate = %v, want a refusal that names the bridge", err)
	}
}

func TestVerifyAccessSavesNothingForATokenThatCannotDoTheWork(t *testing.T) {
	var granted []string
	h := Host{Run: func(_ context.Context, name string, args ...string) ([]byte, error) {
		granted = append(granted, name+" "+strings.Join(args, " "))
		return nil, nil
	}}

	// Fine as it stands: no grant is made, and nothing is asked of Proxmox.
	if err := VerifyAccess(context.Background(), h, "zoomies-abc@pve!provider-1", "Zoomies-abc", "pve", func(context.Context) []string { return nil }); err != nil || len(granted) != 0 {
		t.Fatalf("a healthy token was touched: %v %v", err, granted)
	}

	// Cured by the one grant it is allowed to make: the second check passes.
	calls := 0
	err := VerifyAccess(context.Background(), h, "zoomies-abc@pve!provider-1", "Zoomies-abc", "pve", func(context.Context) []string {
		calls++
		if calls == 1 {
			return []string{"cannot list bridges"}
		}
		return nil
	})
	if err != nil || len(granted) != 4 || !strings.Contains(strings.Join(granted, "\n"), "acl modify /nodes/pve --tokens zoomies-abc@pve!provider-1 --roles Zoomies-abc") || !strings.Contains(strings.Join(granted, "\n"), "acl modify /sdn/zones/localnetwork --tokens zoomies-abc@pve!provider-1 --roles Zoomies-abc") {
		t.Fatalf("a repairable token: err=%v grants=%v", err, granted)
	}

	// Not cured: setup stops and says why, naming the command to look further.
	err = VerifyAccess(context.Background(), h, "zoomies-abc@pve!provider-1", "Zoomies-abc", "pve", func(context.Context) []string { return []string{"cannot list bridges"} })
	if err == nil || !strings.Contains(err.Error(), "cannot list bridges") || !strings.Contains(err.Error(), "pveum user permissions") || !strings.Contains(err.Error(), "nothing was saved") {
		t.Fatalf("an unrepaired token: %v", err)
	}
}

// Privilege separation makes a token's rights the intersection of its own and
// its user's, so a grant that reaches only the token is no grant at all. This
// is what a connection made by an older release relies on to pick up a path a
// newer one needs, since Prepare returns early once a credential exists.
func TestGrantTokenGrantsTheUserAndTheTokenOnEveryPath(t *testing.T) {
	var granted []string
	h := Host{Run: func(_ context.Context, name string, args ...string) ([]byte, error) {
		granted = append(granted, name+" "+strings.Join(args, " "))
		return nil, nil
	}}
	if err := GrantToken(context.Background(), h, "zoomies-abc@pve!provider-1", "Zoomies-abc"); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(granted, "\n")
	for _, path := range []string{"/vms", "/storage", "/nodes", LocalNetworkPath} {
		for _, who := range []string{"--users zoomies-abc@pve", "--tokens zoomies-abc@pve!provider-1"} {
			if !strings.Contains(joined, "pveum acl modify "+path+" "+who+" --roles Zoomies-abc") {
				t.Errorf("no grant of %s on %s:\n%s", who, path, joined)
			}
		}
	}
}

// A template whose /etc/machine-id is missing boots clones that never get an
// address: systemd-networkd's DHCP client cannot start without one. The file
// has to be left empty, which is also how systemd knows to make a fresh id.
func TestImagePreparationLeavesAnEmptyMachineIDForEachCloneToFill(t *testing.T) {
	clean := strings.Index(prepareTemplateImage, "cloud-init clean --logs --machine-id")
	empty := strings.Index(prepareTemplateImage, ": > /etc/machine-id")
	if clean < 0 || empty < 0 || empty < clean {
		t.Fatalf("the machine id is not left empty after the cleanup (cleanup at %d, truncate at %d)", clean, empty)
	}
}
