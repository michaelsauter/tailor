package main

import (
	"fmt"
	"log"
	"os"
	"runtime/debug"

	"github.com/alecthomas/kingpin"
	"github.com/opendevstack/tailor/pkg/cli"
	"github.com/opendevstack/tailor/pkg/commands"
)

var (
	app = kingpin.New(
		"tailor",
		"Tailor - Infrastructure as Code for OpenShift",
	).DefaultEnvars()
	// App-wide flags that cannot be changed in each context
	verboseFlag = app.Flag(
		"verbose",
		"Enable verbose output.",
	).Short('v').Bool()
	debugFlag = app.Flag(
		"debug",
		"Enable debug output (implies verbose).",
	).Short('d').Bool()
	nonInteractiveFlag = app.Flag(
		"non-interactive",
		"Disable interactive mode.",
	).Bool()
	ocBinaryFlag = app.Flag(
		"oc-binary",
		"oc binary to use",
	).Default("oc").String()
	fileFlag = app.Flag(
		"file",
		"Tailorfile with flags.",
	).Short('f').Default("Tailorfile").String()
	contextDirFlag = app.Flag(
		"context-dir",
		"Path to local context directories in which Tailor will execute",
	).Short('c').Default(".").Strings()
	forceFlag = app.Flag(
		"force",
		"Force to continue despite warning (e.g. deleting all resources).",
	).Bool()

	// Context-wide flags which might differ between contexts
	namespaceFlag = app.Flag(
		"namespace",
		"Namespace (omit to use current)",
	).Short('n').String()
	selectorFlag = app.Flag(
		"selector",
		"Selector (label query) to filter on. When using multiple labels (comma-separated), all need to be present (AND condition).",
	).Short('l').String()
	excludeFlag = app.Flag(
		"exclude",
		"Exclude kinds, names and labels (comma separated)",
	).Short('e').String()
	templateDirFlag = app.Flag(
		"template-dir",
		"Path to local templates",
	).Short('t').Default(".").String()
	paramDirFlag = app.Flag(
		"param-dir",
		"Path to parameter files for local templates (defaults to <NAMESPACE> or working directory)",
	).Short('p').Default(".").String()
	publicKeyDirFlag = app.Flag(
		"public-key-dir",
		"Path to public key files",
	).Default(".").String()
	privateKeyFlag = app.Flag(
		"private-key",
		"Path to private key file",
	).Default("private.key").String()
	passphraseFlag = app.Flag(
		"passphrase",
		"Passphrase to unlock key",
	).String()

	versionCommand = app.Command(
		"version",
		"Show version",
	)

	diffCommand = app.Command(
		"diff",
		"Show diff between remote and local",
	).Alias("status")
	diffLabelsFlag = diffCommand.Flag(
		"labels",
		"Label to set in all resources for this template.",
	).String()
	diffParamFlag = diffCommand.Flag(
		"param",
		"Specify a key-value pair (eg. -p FOO=BAR) to set/override a parameter value in the template.",
	).Strings()
	diffParamFileFlag = diffCommand.Flag(
		"param-file",
		"File(s) containing template parameter values to set/override in the template.",
	).Strings()
	diffDiffFlag = diffCommand.Flag(
		"diff",
		"Whether to show textual diff (\"text\") or JSON patches (\"json\"). JSON patches might show secret values in clear text.",
	).Default("text").String()
	diffIgnorePathFlag = diffCommand.Flag(
		"ignore-path",
		"Path(s) per kind/name to ignore (e.g. because they are externally modified) in RFC 6901 format.",
	).PlaceHolder("bc:foobar:/spec/output/to/name").Strings()
	diffIgnoreUnknownParametersFlag = diffCommand.Flag(
		"ignore-unknown-parameters",
		"If true, will not stop processing if a provided parameter does not exist in the template.",
	).Bool()
	diffUpsertOnlyFlag = diffCommand.Flag(
		"upsert-only",
		"Don't delete resource, only create / update.",
	).Short('u').Bool()
	diffAllowRecreateFlag = diffCommand.Flag(
		"allow-recreate",
		"Allow to recreate the whole resource when an immutable field is changed.",
	).Bool()
	diffRevealSecretsFlag = diffCommand.Flag(
		"reveal-secrets",
		"Reveal drift of Secret resources (might show secret values in clear text).",
	).Bool()
	diffResourceArg = diffCommand.Arg(
		"resource", "Remote resource (defaults to all)",
	).String()

	applyCommand = app.Command(
		"apply",
		"Update remote with local",
	).Alias("update")
	applyLabelsFlag = applyCommand.Flag(
		"labels",
		"Label to set in all resources for this template.",
	).String()
	applyParamFlag = applyCommand.Flag(
		"param",
		"Specify a key-value pair (eg. -p FOO=BAR) to set/override a parameter value in the template.",
	).Strings()
	applyParamFileFlag = applyCommand.Flag(
		"param-file",
		"File(s) containing template parameter values to set/override in the template.",
	).Strings()
	applyDiffFlag = applyCommand.Flag(
		"diff",
		"Whether to show textual diff (\"text\") or JSON patches (\"json\"). JSON patches might show secret values in clear text.",
	).Default("text").String()
	applyIgnorePathFlag = applyCommand.Flag(
		"ignore-path",
		"Path(s) per kind to ignore (e.g. because they are externally modified) in RFC 6901 format.",
	).PlaceHolder("bc:foobar:/spec/output/to/name").Strings()
	applyIgnoreUnknownParametersFlag = applyCommand.Flag(
		"ignore-unknown-parameters",
		"If true, will not stop processing if a provided parameter does not exist in the template.",
	).Bool()
	applyUpsertOnlyFlag = applyCommand.Flag(
		"upsert-only",
		"Don't delete resource, only create / apply.",
	).Short('u').Bool()
	applyAllowRecreateFlag = applyCommand.Flag(
		"allow-recreate",
		"Allow to recreate the whole resource when an immutable field is changed.",
	).Bool()
	applyRevealSecretsFlag = applyCommand.Flag(
		"reveal-secrets",
		"Reveal drift of Secret resources (might show secret values in clear text).",
	).Bool()
	applyResourceArg = applyCommand.Arg(
		"resource", "Remote resource (defaults to all)",
	).String()

	exportCommand = app.Command(
		"export",
		"Export remote state as template",
	)
	exportWithAnnotationsFlag = exportCommand.Flag(
		"with-annotations",
		"Export annotations as well.",
	).Bool()
	exportResourceArg = exportCommand.Arg(
		"resource", "Remote resource (defaults to all)",
	).String()

	secretsCommand = app.Command(
		"secrets",
		"Work with secrets",
	)
	editCommand = secretsCommand.Command(
		"edit",
		"Edit param file",
	)
	editFileArg = editCommand.Arg(
		"file", "File to edit",
	).Required().String()

	reEncryptCommand = secretsCommand.Command(
		"re-encrypt",
		"Re-Encrypt param file(s)",
	)
	reEncryptFileArg = reEncryptCommand.Arg(
		"file", "File to re-encrypt",
	).String()

	revealCommand = secretsCommand.Command(
		"reveal",
		"Show param file contents with revealed secrets",
	)
	revealFileArg = revealCommand.Arg(
		"file", "File to show",
	).Required().String()

	generateKeyCommand = secretsCommand.Command(
		"generate-key",
		"Generate new keypair",
	)
	generateKeyNameFlag = generateKeyCommand.Flag(
		"name",
		"Name for keypair",
	).String()
	generateKeyEmailArg = generateKeyCommand.Arg(
		"email", "Emil of keypair",
	).Required().String()
)

func main() {
	defer func() {
		err := recover()
		if err != nil {
			log.Fatalf("Fatal error: %s - %s.", err, debug.Stack())
		}
	}()

	command := kingpin.MustParse(app.Parse(os.Args[1:]))

	if command == versionCommand.FullCommand() {
		fmt.Println("0.11.0+master")
		return
	}

	clusterRequired := true
	if command == editCommand.FullCommand() ||
		command == revealCommand.FullCommand() ||
		command == reEncryptCommand.FullCommand() ||
		command == generateKeyCommand.FullCommand() {
		clusterRequired = false
	}

	globalOptions, err := cli.NewGlobalOptions(
		clusterRequired,
		*fileFlag,
		*verboseFlag,
		*debugFlag,
		*nonInteractiveFlag,
		*ocBinaryFlag,
		*contextDirFlag,
		*forceFlag,
	)
	if err != nil {
		log.Fatalln("Options could not be processed:", err)
	}

	switch command {
	case editCommand.FullCommand():
		secretsOptions, err := cli.NewSecretsOptions(
			globalOptions,
			*paramDirFlag,
			*publicKeyDirFlag,
			*privateKeyFlag,
			*passphraseFlag,
		)
		if err != nil {
			log.Fatalln("Options could not be processed:", err)
		}
		err = commands.Edit(secretsOptions, *editFileArg)
		if err != nil {
			log.Fatalf("Failed to edit file: %s.", err)
		}

	case reEncryptCommand.FullCommand():
		secretsOptions, err := cli.NewSecretsOptions(
			globalOptions,
			*paramDirFlag,
			*publicKeyDirFlag,
			*privateKeyFlag,
			*passphraseFlag,
		)
		if err != nil {
			log.Fatalln("Options could not be processed:", err)
		}
		err = commands.ReEncrypt(secretsOptions, *reEncryptFileArg)
		if err != nil {
			log.Fatalf("Failed to re-encrypt: %s.", err)
		}

	case revealCommand.FullCommand():
		secretsOptions, err := cli.NewSecretsOptions(
			globalOptions,
			*paramDirFlag,
			*publicKeyDirFlag,
			*privateKeyFlag,
			*passphraseFlag,
		)
		if err != nil {
			log.Fatalln("Options could not be processed:", err)
		}
		err = commands.Reveal(secretsOptions, *revealFileArg)
		if err != nil {
			log.Fatalf("Failed to reveal file: %s.", err)
		}

	case generateKeyCommand.FullCommand():
		secretsOptions, err := cli.NewSecretsOptions(
			globalOptions,
			*paramDirFlag,
			*publicKeyDirFlag,
			*privateKeyFlag,
			*passphraseFlag,
		)
		if err != nil {
			log.Fatalln("Options could not be processed:", err)
		}
		err = commands.GenerateKey(secretsOptions, *generateKeyEmailArg, *generateKeyNameFlag)
		if err != nil {
			log.Fatalf("Failed to generate keypair: %s.", err)
		}

	case diffCommand.FullCommand():
		optionSets := map[string]*cli.CompareOptions{}
		for _, contextDir := range globalOptions.ContextDirs {
			opt, err := cli.NewCompareOptions(
				globalOptions,
				contextDir,
				*namespaceFlag,
				*selectorFlag,
				*excludeFlag,
				*templateDirFlag,
				*paramDirFlag,
				*publicKeyDirFlag,
				*privateKeyFlag,
				*passphraseFlag,
				*diffLabelsFlag,
				*diffParamFlag,
				*diffParamFileFlag,
				*diffDiffFlag,
				*diffIgnorePathFlag,
				*diffIgnoreUnknownParametersFlag,
				*diffUpsertOnlyFlag,
				*diffAllowRecreateFlag,
				*diffRevealSecretsFlag,
				*diffResourceArg,
			)
			if err != nil {
				log.Fatalln("Options could not be processed:", err)
			}
			optionSets[contextDir] = opt
		}

		driftDectected, err := commands.Diff(optionSets)
		if err != nil {
			log.Fatalln(err)
		}
		if driftDectected {
			os.Exit(3)
		}

	case applyCommand.FullCommand():
		optionSets := map[string]*cli.CompareOptions{}
		for _, contextDir := range globalOptions.ContextDirs {
			opt, err := cli.NewCompareOptions(
				globalOptions,
				contextDir,
				*namespaceFlag,
				*selectorFlag,
				*excludeFlag,
				*templateDirFlag,
				*paramDirFlag,
				*publicKeyDirFlag,
				*privateKeyFlag,
				*passphraseFlag,
				*applyLabelsFlag,
				*applyParamFlag,
				*applyParamFileFlag,
				*applyDiffFlag,
				*applyIgnorePathFlag,
				*applyIgnoreUnknownParametersFlag,
				*applyUpsertOnlyFlag,
				*applyAllowRecreateFlag,
				*applyRevealSecretsFlag,
				*applyResourceArg,
			)
			if err != nil {
				log.Fatalln("Options could not be processed:", err)
			}
			optionSets[contextDir] = opt
		}

		err = commands.Apply(globalOptions.NonInteractive, optionSets)
		if err != nil {
			log.Fatalln(err)
		}

	case exportCommand.FullCommand():
		optionSets := map[string]*cli.ExportOptions{}
		for _, contextDir := range globalOptions.ContextDirs {
			opt, err := cli.NewExportOptions(
				globalOptions,
				contextDir,
				*namespaceFlag,
				*selectorFlag,
				*excludeFlag,
				*templateDirFlag,
				*paramDirFlag,
				*exportWithAnnotationsFlag,
				*exportResourceArg,
			)
			if err != nil {
				log.Fatalln("Options could not be processed:", err)
			}
			optionSets[contextDir] = opt
		}
		err = commands.Export(optionSets)
		if err != nil {
			log.Fatalln(err)
		}
	}
}
