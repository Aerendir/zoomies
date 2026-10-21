package aicontext

import (
	"embed"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

const (
	GeneratorPackagePath = ".github/zoomies-ai-context/package.json"
	GeneratorLockPath    = ".github/zoomies-ai-context/package-lock.json"
)

//go:embed templates/*
var setupTemplates embed.FS

func SetupWorkflow(key RepositoryKey, config Config) (string, error) {
	return setupWorkflowWithPins(key, config, nil)
}

// setupWorkflowWithPins builds the managed workflow with a set of action
// versions in place of the ones the template carries. A nil set is the
// template's own versions.
func setupWorkflowWithPins(key RepositoryKey, config Config, pins map[string]string) (string, error) {
	if err := key.Validate(); err != nil {
		return "", err
	}
	if err := config.Validate(); err != nil {
		return "", err
	}
	// GHE.com is laid out like GitHub.com but signs Actions tokens with its own
	// issuer and has not been tried; refuse it rather than guess, so nothing
	// is merged that would only fail afterwards.
	if key.GitHubHost == "ghe.com" || strings.HasSuffix(key.GitHubHost, ".ghe.com") {
		return "", fmt.Errorf("managed context workflows are not available for GHE.com yet; use a repository on GitHub.com or GitHub Enterprise Server")
	}
	cfg, _ := json.Marshal(managedConfig{Manager: "zoomies-ai-context", TemplateVersion: SetupTemplateVersion, Config: config})
	identity, _ := json.Marshal(key)
	hash, _ := config.Hash()
	branch, _ := json.Marshal(config.SourceBranch)
	pushBranch, _ := json.Marshal(strings.NewReplacer("!", `\!`, "+", `\+`).Replace(config.SourceBranch))
	generate, _ := setupTemplates.ReadFile("templates/generate.py")
	publish, _ := setupTemplates.ReadFile("templates/publish.py")
	upload, _ := setupTemplates.ReadFile("templates/upload.py")
	quote := func(s string) string { b, _ := json.Marshal(s); return string(b) }
	workflow := `# Managed by Zoomies AI Context; template 1. Review upgrades through a PR.
name: Zoomies AI Context
on:
  push:
    branches: [@@PUSH_BRANCH@@]
  workflow_dispatch:
permissions:
  contents: read
concurrency:
  group: zoomies-ai-context
  cancel-in-progress: true
jobs:
  generate:
    if: github.ref == TRUSTED_REF && (github.event_name == 'push' || github.event_name == 'workflow_dispatch')
    runs-on: ubuntu-latest
    timeout-minutes: 15
    permissions:
      contents: read
    steps:
      - uses: actions/checkout@3d3c42e5aac5ba805825da76410c181273ba90b1 # v7.0.1
        with:
          ref: ${{ github.sha }}
          path: source
          persist-credentials: false
          submodules: false
          lfs: false
      - uses: actions/setup-node@949feb2413d6458794dcd2491c4babbbce0c15c1 # v7.1.0
        with:
          node-version: '24'
          package-manager-cache: false
      - name: Install pinned generator without repository scripts
        shell: bash
        run: |
          set -eu
          mkdir -p "$RUNNER_TEMP/zoomies-tools"
          cp source/.github/zoomies-ai-context/package.json "$RUNNER_TEMP/zoomies-tools/package.json"
          cp source/.github/zoomies-ai-context/package-lock.json "$RUNNER_TEMP/zoomies-tools/package-lock.json"
          npm ci --prefix "$RUNNER_TEMP/zoomies-tools" --ignore-scripts --no-audit --no-fund > /dev/null 2>&1
      - name: Generate bounded source context
        env:
          SOURCE_DIR: ${{ github.workspace }}/source
          OUTPUT_DIR: ${{ runner.temp }}/zoomies-context
          REPOMIX_CLI: ${{ runner.temp }}/zoomies-tools/node_modules/.bin/repomix
          SOURCE_COMMIT: ${{ github.sha }}
          EXPECTED_CONFIG: @@CONFIG@@
          REPOSITORY_IDENTITY: @@IDENTITY@@
          CONFIG_HASH: @@HASH@@
        shell: python
        run: |
@@GENERATE@@
      - uses: actions/upload-artifact@cf430e030ddbb5b0abf93d22962f4752f3646cd9 # v7.0.2
        with:
          name: zoomies-ai-context
          path: ${{ runner.temp }}/zoomies-context/
          if-no-files-found: error
          retention-days: 1
@@DELIVER@@`
	// Zoomies-only output never writes to the repository: its last job holds
	// an OIDC token instead of contents: write, and sends the snapshot to the
	// controller named in the reviewed configuration.
	deliver := `  publish:
    needs: generate
    if: github.ref == TRUSTED_REF
    runs-on: ubuntu-latest
    timeout-minutes: 10
    permissions:
      contents: write
    steps:
      - uses: actions/download-artifact@9000827ccba6bdab643e8b6fd33ac0654aef8333 # v8.0.2
        with:
          name: zoomies-ai-context
          path: ${{ runner.temp }}/zoomies-context
      - name: Validate and atomically publish the generated branch
        env:
          GH_TOKEN: ${{ github.token }}
          ARTIFACT_DIR: ${{ runner.temp }}/zoomies-context
          SOURCE_COMMIT: ${{ github.sha }}
          SOURCE_BRANCH: @@BRANCH@@
          REPOSITORY_IDENTITY: @@IDENTITY@@
          CONFIG_HASH: @@HASH@@
        shell: python
        run: |
@@PUBLISH@@
`
	if config.Destination == Zoomies {
		deliver = `  upload:
    needs: generate
    if: github.ref == TRUSTED_REF
    runs-on: ubuntu-latest
    timeout-minutes: 10
    permissions:
      id-token: write
    steps:
      - uses: actions/download-artifact@9000827ccba6bdab643e8b6fd33ac0654aef8333 # v8.0.2
        with:
          name: zoomies-ai-context
          path: ${{ runner.temp }}/zoomies-context
      - name: Upload the verified context to Zoomies
        env:
          ARTIFACT_DIR: ${{ runner.temp }}/zoomies-context
          SOURCE_COMMIT: ${{ github.sha }}
          SOURCE_BRANCH: @@BRANCH@@
          REPOSITORY_IDENTITY: @@IDENTITY@@
          CONFIG_HASH: @@HASH@@
          UPLOAD_URL: @@UPLOAD_URL@@
        shell: python
        run: |
@@UPLOAD@@
`
	}
	workflow = strings.Replace(workflow, "@@DELIVER@@", deliver, 1)
	// GitHub expressions need a single-quoted literal, with apostrophes
	// doubled; JSON/YAML quoting protects the surrounding YAML scalar.
	trustedRef := quote("${{ github.ref == 'refs/heads/" + strings.ReplaceAll(config.SourceBranch, "'", "''") + "' }}")
	condition := "${{ github.ref == 'refs/heads/" + strings.ReplaceAll(config.SourceBranch, "'", "''") + "' && (github.event_name == 'push' || github.event_name == 'workflow_dispatch') }}"
	workflow = strings.ReplaceAll(workflow, "github.ref == TRUSTED_REF && (github.event_name == 'push' || github.event_name == 'workflow_dispatch')", quote(condition))
	workflow = strings.ReplaceAll(workflow, "github.ref == TRUSTED_REF", trustedRef)
	// The action versions go in before the Enterprise Server rewrite, which
	// replaces the actions by name: an earlier release's versions are
	// rewritten to the Enterprise ones in the same way. The Enterprise versions
	// have not moved, so the Enterprise workflow is the same either way.
	if pins != nil {
		workflow = withActionPins(workflow, pins)
	}
	if key.GitHubHost != "github.com" {
		workflow = enterpriseServerWorkflow(workflow)
	}
	workflow = strings.NewReplacer("@@PUSH_BRANCH@@", string(pushBranch), "@@BRANCH@@", string(branch), "@@CONFIG@@", quote(string(cfg)), "@@IDENTITY@@", quote(string(identity)), "@@HASH@@", quote(hash), "@@GENERATE@@", indentScript(string(generate)), "@@PUBLISH@@", indentScript(string(publish)), "@@UPLOAD_URL@@", quote(config.UploadURL), "@@UPLOAD@@", indentScript(string(upload))).Replace(workflow)
	return workflow, nil
}

// enterpriseActionPins are the versions Enterprise Server bundles, by action.
var enterpriseActionPins = map[string]string{
	"actions/checkout":          "actions/checkout@11d5960a326750d5838078e36cf38b85af677262 # v4.4.0",
	"actions/setup-node":        "actions/setup-node@49933ea5288caeca8642d1e84afbd3f7d6820020 # v4.4.0",
	"actions/upload-artifact":   "actions/upload-artifact@c6a366c94c3e0affe28c06c8df20a878f24da3cf # v3.2.2",
	"actions/download-artifact": "actions/download-artifact@a9bc5e6ef2cb54c177f32aa5726adaa15e7e2d59 # v3.1.0",
}

// enterpriseServerWorkflow rewrites the GitHub.com workflow for GitHub
// Enterprise Server. Artifact actions v4 and later do not run there, so the
// jobs hand the context over with v3 -- which keeps the generator in a job
// that cannot write to the repository -- and every action is pinned to the
// release Enterprise Server bundles, so the workflow runs without GitHub
// Connect. Enterprise Server has no GitHub-hosted runners: the jobs ask for
// any self-hosted Linux runner, which is a Zoomies pool.
//
// The actions are replaced by name rather than by the version the template
// happens to hold, so this works the same for the template's own versions and
// for an earlier release's, which the recognition of an installed workflow
// needs.
func enterpriseServerWorkflow(workflow string) string {
	workflow = strings.ReplaceAll(workflow, "runs-on: ubuntu-latest", "runs-on: [self-hosted, linux]")
	workflow = workflowActionUse.ReplaceAllStringFunc(workflow, func(line string) string {
		m := workflowActionUse.FindStringSubmatch(line)
		pin, ok := enterpriseActionPins[m[workflowActionUse.SubexpIndex("action")]]
		if !ok {
			return line
		}
		return m[1] + pin
	})
	// setup-node v4 has no package-manager-cache input, and caches nothing
	// unless asked, which is what the GitHub.com workflow sets it to.
	return strings.ReplaceAll(workflow, "          package-manager-cache: false\n", "")
}

var workflowActionUse = regexp.MustCompile(`(?m)^(?P<prefix>[ \t]*(?:-[ \t]+)?uses:[ \t]+)(?P<action>[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+)@(?P<ref>[^ \t\r\n]+)(?P<trail>[^\r\n]*)$`)

// keepInstalledActionPins leaves each action at the version the repository
// already has. Regenerating the workflow around them is the repair; silently
// moving a pin backwards, even to one this release ships, is a change nobody
// reviewed and can undo a security update the repository had taken.
func keepInstalledActionPins(installed, generated string) string {
	pins := map[string]string{}
	for _, m := range workflowActionUse.FindAllStringSubmatch(installed, -1) {
		pins[m[workflowActionUse.SubexpIndex("action")]] = m[workflowActionUse.SubexpIndex("ref")] + m[workflowActionUse.SubexpIndex("trail")]
	}
	if len(pins) == 0 {
		return generated
	}
	return workflowActionUse.ReplaceAllStringFunc(generated, func(line string) string {
		m := workflowActionUse.FindStringSubmatch(line)
		action := m[workflowActionUse.SubexpIndex("action")]
		pin, ok := pins[action]
		if !ok {
			return line
		}
		return m[workflowActionUse.SubexpIndex("prefix")] + action + "@" + pin
	})
}

func indentScript(s string) string {
	lines := strings.Split(strings.TrimSuffix(s, "\n"), "\n")
	for i, line := range lines {
		if line != "" {
			lines[i] = "          " + line
		}
	}
	return strings.Join(lines, "\n")
}

// LegacySetupWorkflow recognises the exact initial template during upgrades;
// arbitrary changes still close the ingestion gate.
func LegacySetupWorkflow(key RepositoryKey, config Config) (string, error) {
	return legacySetupWorkflowWithPins(key, config, nil)
}

// legacySetupWorkflowWithPins is the initial template with a set of action
// versions in place of the template's own.
func legacySetupWorkflowWithPins(key RepositoryKey, config Config, pins map[string]string) (string, error) {
	w, err := setupWorkflowWithPins(key, config, pins)
	if err != nil {
		return "", err
	}
	for _, name := range []string{"generate", "publish"} {
		current, _ := setupTemplates.ReadFile("templates/" + name + ".py")
		legacy, _ := setupTemplates.ReadFile("templates/legacy_" + name + ".py")
		oldIndent := "          " + strings.ReplaceAll(strings.TrimSuffix(string(legacy), "\n"), "\n", "\n          ")
		w = strings.Replace(w, indentScript(string(current)), oldIndent, 1)
	}
	return w, nil
}

// olderGenerations are the generator and publisher scripts earlier releases put
// in installed workflows, newest first. Installed workflows are verified byte
// for byte, so a template change that left them unrecognised would close the
// access gate of every enabled repository until somebody ran a repair. Add the
// scripts being replaced here, in the same change that replaces them; the
// initial template is handled by LegacySetupWorkflow.
var olderGenerations = []struct{ generate, publish string }{
	// Named oversized files, but refused the run when there was one.
	{"templates/named_generate.py", "templates/older_publish.py"},
	// Said only that "a source file exceeds the context size limit".
	{"templates/previous_generate.py", "templates/older_publish.py"},
}

// olderActionPins are the action versions earlier releases wrote into an
// installed workflow, newest first. A workflow is recognised by the pins it
// carries as well as by the scripts it runs, so taking a new version of an
// action does not read as somebody's edit: it would close the access gate of
// every repository that had installed the earlier version, and the only way
// back would be a repair nobody asked for. Add the pins being replaced here,
// in the same change that replaces them.
//
// The set is written for GitHub.com, which is what the template holds before
// the Enterprise Server rewrite; the Enterprise Server versions are the ones
// that rewrite has always put there, and they have not moved.
var olderActionPins = []map[string]string{
	// The versions before the actions were taken up to their current ones.
	{
		"actions/checkout":          "3d3c42e5aac5ba805825da76410c181273ba90b1 # v7.0.1",
		"actions/setup-node":        "820762786026740c76f36085b0efc47a31fe5020 # v7.0.0",
		"actions/upload-artifact":   "043fb46d1a93c77aae656e7c1c64a875d1fc6a0a # v7.0.1",
		"actions/download-artifact": "634f93cb2916e3fdff6788551b99b062d0335ce0 # v5",
	},
}

// withActionPins puts a set of action versions into a workflow, replacing the
// ones it carries now. The set is by action, so a workflow that uses an
// action the set does not mention is left as it is.
func withActionPins(workflow string, pins map[string]string) string {
	return workflowActionUse.ReplaceAllStringFunc(workflow, func(line string) string {
		m := workflowActionUse.FindStringSubmatch(line)
		pin, ok := pins[m[workflowActionUse.SubexpIndex("action")]]
		if !ok {
			return line
		}
		return m[workflowActionUse.SubexpIndex("prefix")] + m[workflowActionUse.SubexpIndex("action")] + "@" + pin
	})
}

func olderSetupWorkflow(key RepositoryKey, config Config, generation int) (string, error) {
	return olderSetupWorkflowWithPins(key, config, generation, nil)
}

// olderSetupWorkflowWithPins is the workflow of an earlier script generation
// with a set of action versions in place of the template's own.
func olderSetupWorkflowWithPins(key RepositoryKey, config Config, generation int, pins map[string]string) (string, error) {
	w, err := setupWorkflowWithPins(key, config, pins)
	if err != nil {
		return "", err
	}
	g := olderGenerations[generation]
	for _, swap := range [][2]string{{"templates/generate.py", g.generate}, {"templates/publish.py", g.publish}} {
		current, _ := setupTemplates.ReadFile(swap[0])
		older, _ := setupTemplates.ReadFile(swap[1])
		w = strings.Replace(w, indentScript(string(current)), indentScript(string(older)), 1)
	}
	return w, nil
}

// PreviousSetupWorkflow is the template immediately before the current one.
func PreviousSetupWorkflow(key RepositoryKey, config Config) (string, error) {
	return olderSetupWorkflow(key, config, 0)
}

// IsOlderSetupWorkflow reports whether content is exactly a workflow an earlier
// Zoomies release wrote for this repository and configuration. Such a workflow
// is still owned and still safe; it is merely due a repair. Anything else is
// somebody's edit.
//
// The action versions count as part of what an earlier release wrote: a
// repository that installed the workflow before an action was taken up to a
// newer version still owns what it installed, and reading it as an edit would
// close its access gate.
func IsOlderSetupWorkflow(key RepositoryKey, config Config, content string) bool {
	// The action versions an earlier release wrote are the GitHub.com ones:
	// the Enterprise Server workflow has always carried the versions that
	// server bundles, and those have not moved.
	pins := olderActionPins
	if key.GitHubHost != "github.com" {
		pins = nil
	}
	// The scripts the template runs now, with the action versions it used to
	// carry: a release that took a new version of an action changed nothing
	// else in the workflow.
	for _, set := range pins {
		w, err := setupWorkflowWithPins(key, config, set)
		if err == nil && w == content {
			return true
		}
	}
	for i := range olderGenerations {
		for _, set := range append([]map[string]string{nil}, pins...) {
			w, err := olderSetupWorkflowWithPins(key, config, i, set)
			if err == nil && w == content {
				return true
			}
		}
	}
	for _, set := range append([]map[string]string{nil}, pins...) {
		w, err := legacySetupWorkflowWithPins(key, config, set)
		if err == nil && w == content {
			return true
		}
	}
	return false
}

// olderGeneratorLocks are the generator lockfiles earlier releases put in
// installed repositories, newest first. An installed lockfile is verified byte
// for byte like the workflow, so a template change that left the one it replaced
// unrecognised would read every installed copy as somebody's edit and close the
// access gate of its repository until somebody ran a repair. Keep the file being
// replaced here, in the same change that replaces it. They are kept under a name
// that is not package-lock.json, because nothing installs them and a dependency
// scanner has no reason to read a tree that exists only to be recognised.
var olderGeneratorLocks = []string{
	// handlebars 4.7.9, which carried two critical advisories; 4.7.10 is the fix.
	"templates/older_package-lock.json",
}

// PreviousGeneratorLock is the lockfile immediately before the current one.
func PreviousGeneratorLock() (string, error) {
	older, err := setupTemplates.ReadFile(olderGeneratorLocks[0])
	return string(older), err
}

// IsOlderGeneratorLock reports whether content is exactly a lockfile an earlier
// Zoomies release wrote. Such a file is still owned and still installs the
// version of Repomix it names; it is merely due a repair.
func IsOlderGeneratorLock(content string) bool {
	for _, name := range olderGeneratorLocks {
		if older, err := setupTemplates.ReadFile(name); err == nil && string(older) == content {
			return true
		}
	}
	return false
}

// PlanManagedSetup adds executable workflow/toolchain files only when they are
// absent or byte-for-byte owned. User edits never become silent upgrades.
func PlanManagedSetup(key RepositoryKey, name string, config Config, files []SetupFile) ([]SetupChange, error) {
	workflow, err := SetupWorkflow(key, config)
	if err != nil {
		return nil, err
	}
	lock, _ := setupTemplates.ReadFile("templates/package-lock.json")
	packageJSON := `{"name":"zoomies-ai-context","private":true,"dependencies":{"repomix":"1.18.1"}}` + "\n"
	desired := map[string]string{WorkflowPath: workflow, GeneratorPackagePath: packageJSON, GeneratorLockPath: string(lock)}
	basic := make([]SetupFile, 0, len(files))
	seen := map[string]bool{}
	for _, file := range files {
		if file.Mode != "" && file.Mode != "100644" && file.Mode != "100755" {
			return nil, fmt.Errorf("setup file %q must be a regular file", file.Path)
		}
		if seen[file.Path] {
			return nil, fmt.Errorf("setup includes duplicate file %q", file.Path)
		}
		seen[file.Path] = true
		if want, ok := desired[file.Path]; ok {
			if file.SHA == "" && file.Content != "" || file.SHA != "" && !commitPattern.MatchString(file.SHA) {
				return nil, fmt.Errorf("setup file %q needs a valid blob identity", file.Path)
			}
			if file.SHA != "" && file.Content != want {
				return nil, fmt.Errorf("%s already exists with different contents; review a repair or upgrade", file.Path)
			}
			if file.SHA != "" {
				delete(desired, file.Path)
			}
		} else {
			basic = append(basic, file)
		}
	}
	changes, err := PlanSetupFiles(key, name, config, basic)
	if err != nil {
		return nil, err
	}
	for _, p := range []string{WorkflowPath, GeneratorPackagePath, GeneratorLockPath} {
		if content, ok := desired[p]; ok {
			changes = append(changes, SetupChange{Mode: "100644", Path: p, Content: content})
		}
	}
	return changes, nil
}
