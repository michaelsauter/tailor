package main

import (
	"errors"
	"fmt"
	"github.com/alecthomas/kingpin"
	"github.com/ghodss/yaml"
	"github.com/opendevstack/tailor/cli"
	"github.com/opendevstack/tailor/openshift"
	"github.com/opendevstack/tailor/utils"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"regexp"
	"runtime/debug"
	"sort"
	"strings"
)

var (
	app = kingpin.New(
		"tailor",
		"Tailor - Infrastructure as Code for OpenShift",
	).DefaultEnvars()
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
	fileFlag = app.Flag(
		"file",
		"Tailorfile with flags.",
	).Short('f').Default("Tailorfile").String()

	namespaceFlag = app.Flag(
		"namespace",
		"Namespace (omit to use current)",
	).Short('n').String()
	selectorFlag = app.Flag(
		"selector",
		"Selector (label query) to filter on",
	).Short('l').String()
	templateDirFlag = app.Flag(
		"template-dir",
		"Path to local templates",
	).Short('t').Default(".").Strings()
	paramDirFlag = app.Flag(
		"param-dir",
		"Path to param files for local templates",
	).Short('p').Default(".").Strings()
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

	statusCommand = app.Command(
		"status",
		"Show diff between remote and local",
	)
	statusLabelsFlag = statusCommand.Flag(
		"labels",
		"Label to set in all resources for this template.",
	).String()
	statusParamFlag = statusCommand.Flag(
		"param",
		"Specify a key-value pair (eg. -p FOO=BAR) to set/override a parameter value in the template.",
	).Strings()
	statusParamFileFlag = statusCommand.Flag(
		"param-file",
		"File containing template parameter values to set/override in the template.",
	).String()
	statusIgnoreUnknownParametersFlag = statusCommand.Flag(
		"ignore-unknown-parameters",
		"If true, will not stop processing if a provided parameter does not exist in the template.",
	).Bool()
	statusUpsertOnlyFlag = statusCommand.Flag(
		"upsert-only",
		"Don't delete resource, only create / update.",
	).Short('u').Bool()
	statusResourceArg = statusCommand.Arg(
		"resource", "Remote resource (defaults to all)",
	).String()

	updateCommand = app.Command(
		"update",
		"Update remote with local",
	)
	updateLabelsFlag = updateCommand.Flag(
		"labels",
		"Label to set in all resources for this template.",
	).String()
	updateParamFlag = updateCommand.Flag(
		"param",
		"Specify a key-value pair (eg. -p FOO=BAR) to set/override a parameter value in the template.",
	).Strings()
	updateParamFileFlag = updateCommand.Flag(
		"param-file",
		"File containing template parameter values to set/override in the template.",
	).String()
	updateIgnoreUnknownParametersFlag = updateCommand.Flag(
		"ignore-unknown-parameters",
		"If true, will not stop processing if a provided parameter does not exist in the template.",
	).Bool()
	updateUpsertOnlyFlag = updateCommand.Flag(
		"upsert-only",
		"Don't delete resource, only create / update.",
	).Short('u').Bool()
	updateResourceArg = updateCommand.Arg(
		"resource", "Remote resource (defaults to all)",
	).String()

	exportCommand = app.Command(
		"export",
		"Export remote state as template",
	)
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

	kindMapping = map[string]string{
		"svc":              "Service",
		"service":          "Service",
		"route":            "Route",
		"dc":               "DeploymentConfig",
		"deploymentconfig": "DeploymentConfig",
		"bc":               "BuildConfig",
		"buildconfig":      "BuildConfig",
		"is":               "ImageStream",
		"imagestream":      "ImageStream",
		"pvc":              "PersistentVolumeClaim",
		"persistentvolumeclaim": "PersistentVolumeClaim",
		"template":              "Template",
		"cm":                    "ConfigMap",
		"configmap":             "ConfigMap",
		"secret":                "Secret",
		"rolebinding":           "RoleBinding",
		"serviceaccount":        "ServiceAccount",
	}
)

func main() {
	defer func() {
		err := recover()
		if err != nil {
			log.Fatalf("Fatal error: %s - %s.", err, debug.Stack())
		}
	}()

	command := kingpin.MustParse(app.Parse(os.Args[1:]))

	fileFlags, err := cli.GetFileFlags(*fileFlag, (*verboseFlag || *debugFlag))
	if err != nil {
		log.Fatalln("Could not read Tailorfile:", err)
	}
	globalOptions := &cli.GlobalOptions{}
	globalOptions.UpdateWithFile(fileFlags)
	globalOptions.UpdateWithFlags(
		*verboseFlag,
		*debugFlag,
		*nonInteractiveFlag,
		*namespaceFlag,
		*selectorFlag,
		*templateDirFlag,
		*paramDirFlag,
		*publicKeyDirFlag,
		*privateKeyFlag,
		*passphraseFlag,
	)
	err = globalOptions.Process()
	if err != nil {
		log.Fatalln("Options could not be processed:", err)
	}

	switch command {
	case versionCommand.FullCommand():
		fmt.Println("0.7.0")

	case editCommand.FullCommand():
		readParams, err := openshift.NewParamsFromFile(*editFileArg, globalOptions.PrivateKey, globalOptions.Passphrase)
		if err != nil {
			log.Fatalf("Could not read file: %s.", err)
		}
		readContent, _ := readParams.Process(false, false)

		editedContent, err := cli.EditEnvFile(readContent)
		if err != nil {
			log.Fatalf("Could not edit file: %s.", err)
		}
		editedParams, err := openshift.NewParamsFromInput(editedContent)
		if err != nil {
			log.Fatal(err)
		}

		renderedContent, err := editedParams.Render(globalOptions.PublicKeyDir, readParams)
		if err != nil {
			log.Fatal(err)
		}
		err = ioutil.WriteFile(*editFileArg, []byte(renderedContent), 0644)
		if err != nil {
			log.Fatalf("Could not write file: %s.", err)
		}

	case reEncryptCommand.FullCommand():
		if len(*reEncryptFileArg) > 0 {
			err := reEncrypt(*reEncryptFileArg, globalOptions.PrivateKey, globalOptions.Passphrase, globalOptions.PublicKeyDir)
			if err != nil {
				log.Fatal(err)
			}
		} else {
			for _, paramDir := range globalOptions.ParamDirs {
				files, err := ioutil.ReadDir(paramDir)
				if err != nil {
					log.Fatal(err)
				}
				filePattern := ".*\\.env$"
				for _, file := range files {
					matched, _ := regexp.MatchString(filePattern, file.Name())
					if !matched {
						continue
					}
					filename := paramDir + string(os.PathSeparator) + file.Name()
					err := reEncrypt(filename, globalOptions.PrivateKey, globalOptions.Passphrase, globalOptions.PublicKeyDir)
					if err != nil {
						log.Fatal(err)
					}
				}
			}
		}

	case revealCommand.FullCommand():
		if _, err := os.Stat(*revealFileArg); os.IsNotExist(err) {
			log.Fatalf("'%s' does not exist.", *revealFileArg)
		}
		readParams, err := openshift.NewParamsFromFile(*revealFileArg, globalOptions.PrivateKey, globalOptions.Passphrase)
		if err != nil {
			log.Fatalf("Could not read file: %s.", err)
		}
		readContent, err := readParams.Process(false, true)
		if err != nil {
			log.Fatalf("Failed to process: %s.", err)
		}
		fmt.Println(readContent)

	case generateKeyCommand.FullCommand():
		emailParts := strings.Split(*generateKeyEmailArg, "@")
		name := *generateKeyNameFlag
		if len(name) == 0 {
			name = emailParts[0]
		}
		entity, err := utils.CreateEntity(name, *generateKeyEmailArg)
		if err != nil {
			log.Fatalf("Failed to generate keypair: %s.", err)
		}
		publicKeyFilename := strings.Replace(emailParts[0], ".", "-", -1) + ".key"
		utils.PrintPublicKey(entity, publicKeyFilename)
		if err != nil {
			log.Fatalln(err)
		}
		fmt.Printf("Public Key written to %s. This file can be committed.\n", publicKeyFilename)
		privateKeyFilename := globalOptions.PrivateKey
		utils.PrintPrivateKey(entity, privateKeyFilename)
		if err != nil {
			log.Fatalln(err)
		}
		fmt.Printf("Private Key written to %s. This file MUST NOT be committed.\n", privateKeyFilename)

	case statusCommand.FullCommand():
		checkLoggedIn()

		compareOptions := &cli.CompareOptions{
			GlobalOptions: globalOptions,
		}
		compareOptions.UpdateWithFile(fileFlags)
		compareOptions.UpdateWithFlags(
			*statusLabelsFlag,
			*statusParamFlag,
			*statusParamFileFlag,
			*statusIgnoreUnknownParametersFlag,
			*statusUpsertOnlyFlag,
			*statusResourceArg,
		)
		err := compareOptions.Process()
		if err != nil {
			log.Fatalln("Options could not be processed:", err)
		}

		updateRequired, _, err := calculateChangeset(compareOptions)
		if err != nil {
			log.Fatalln(err)
		}

		if updateRequired {
			os.Exit(3)
		}

	case updateCommand.FullCommand():
		checkLoggedIn()

		compareOptions := &cli.CompareOptions{
			GlobalOptions: globalOptions,
		}
		compareOptions.UpdateWithFile(fileFlags)
		compareOptions.UpdateWithFlags(
			*updateLabelsFlag,
			*updateParamFlag,
			*updateParamFileFlag,
			*updateIgnoreUnknownParametersFlag,
			*updateUpsertOnlyFlag,
			*updateResourceArg,
		)
		err := compareOptions.Process()
		if err != nil {
			log.Fatalln("Options could not be processed:", err)
		}

		updateRequired, changeset, err := calculateChangeset(compareOptions)
		if err != nil {
			log.Fatalln(err)
		}

		if updateRequired {
			if globalOptions.NonInteractive {
				openshift.UpdateRemote(changeset, compareOptions)
			} else {
				c := cli.AskForConfirmation("Apply changes?")
				if c {
					openshift.UpdateRemote(changeset, compareOptions)
				}
			}
		}

	case exportCommand.FullCommand():
		checkLoggedIn()

		exportOptions := &cli.ExportOptions{
			GlobalOptions: globalOptions,
		}
		exportOptions.UpdateWithFile(fileFlags)
		exportOptions.UpdateWithFlags(
			*exportResourceArg,
		)

		filter, err := getFilter(exportOptions.Resource, exportOptions.Selector)
		if err != nil {
			log.Fatalln(err)
		}
		export(filter, exportOptions)
	}
}

func reEncrypt(filename, privateKey, passphrase, publicKeyDir string) error {
	readParams, err := openshift.NewParamsFromFile(filename, privateKey, passphrase)
	if err != nil {
		return errors.New(fmt.Sprintf("Could not read file: %s.", err))
	}
	readContent, _ := readParams.Process(false, false)

	editedParams, err := openshift.NewParamsFromInput(readContent)
	if err != nil {
		return err
	}

	renderedContent, err := editedParams.Render(publicKeyDir, []*openshift.Param{})
	if err != nil {
		return err
	}
	err = ioutil.WriteFile(filename, []byte(renderedContent), 0644)
	if err != nil {
		return errors.New(fmt.Sprintf("Could not write file: %s.", err))
	}
	return nil
}

func calculateChangeset(compareOptions *cli.CompareOptions) (bool, *openshift.Changeset, error) {
	updateRequired := false

	where := strings.Join(compareOptions.TemplateDirs, ", ")
	if len(compareOptions.TemplateDirs) == 1 && compareOptions.TemplateDirs[0] == "." {
		where, _ = os.Getwd()
	}

	fmt.Printf(
		"Comparing templates in %s with OCP namespace %s.\n",
		where,
		compareOptions.Namespace,
	)

	if len(compareOptions.Resource) > 0 && len(compareOptions.Selector) > 0 {
		fmt.Printf(
			"Limiting resources to %s with selector %s.\n",
			compareOptions.Resource,
			compareOptions.Selector,
		)
	} else if len(compareOptions.Selector) > 0 {
		fmt.Printf(
			"Limiting to resources with selector %s.\n",
			compareOptions.Selector,
		)
	} else if len(compareOptions.Resource) > 0 {
		fmt.Printf(
			"Limiting resources to %s.\n",
			compareOptions.Resource,
		)
	}

	resource := compareOptions.Resource
	selectorFlag := compareOptions.Selector
	upsertOnly := compareOptions.UpsertOnly

	filter, err := getFilter(resource, selectorFlag)
	if err != nil {
		return updateRequired, &openshift.Changeset{}, err
	}

	localResourceList := assembleLocalResourceList(
		filter,
		compareOptions,
	)
	remoteResourceList := assembleRemoteResourceList(filter, compareOptions)
	remoteResourcesWord := "resources"
	if remoteResourceList.Length() == 1 {
		remoteResourcesWord = "resource"
	}
	localResourcesWord := "resources"
	if localResourceList.Length() == 1 {
		localResourcesWord = "resource"
	}
	fmt.Printf(
		"Found %d %s in OCP cluster (current state) and %d %s in processed templates (desired state).\n\n",
		remoteResourceList.Length(),
		remoteResourcesWord,
		localResourceList.Length(),
		localResourcesWord,
	)

	changeset := compare(remoteResourceList, localResourceList, upsertOnly)
	updateRequired = !changeset.Blank()
	return updateRequired, changeset, nil
}

// kindArgs might be blank, or a list of kinds (e.g. 'pvc,dc') or
// a kind/name combination (e.g. 'dc/foo').
// selectorFlag might be blank or a key and a label, e.g. 'name=foo'.
func getFilter(kindArg string, selectorFlag string) (*openshift.ResourceFilter, error) {
	filter := &openshift.ResourceFilter{
		Kinds: []string{},
		Name:  "",
		Label: selectorFlag,
	}

	if len(kindArg) == 0 {
		return filter, nil
	}

	kindArg = strings.ToLower(kindArg)

	if strings.Contains(kindArg, "/") {
		if strings.Contains(kindArg, ",") {
			return nil, errors.New(
				"You cannot target more than one resource name",
			)
		}
		nameParts := strings.Split(kindArg, "/")
		filter.Name = kindMapping[nameParts[0]] + "/" + nameParts[1]
		return filter, nil
	}

	targetedKinds := make(map[string]bool)
	unknownKinds := []string{}
	kinds := strings.Split(kindArg, ",")
	for _, kind := range kinds {
		if _, ok := kindMapping[kind]; !ok {
			unknownKinds = append(unknownKinds, kind)
		} else {
			targetedKinds[kindMapping[kind]] = true
		}
	}

	if len(unknownKinds) > 0 {
		return nil, errors.New(fmt.Sprintf(
			"Unknown resource kinds: %s",
			strings.Join(unknownKinds, ","),
		))
	}

	for kind, _ := range targetedKinds {
		filter.Kinds = append(filter.Kinds, kind)
	}

	sort.Strings(filter.Kinds)

	return filter, nil
}

func checkLoggedIn() {
	cmd := exec.Command("oc", "whoami")
	_, err := cmd.CombinedOutput()
	if err != nil {
		log.Fatalln("You need to login with 'oc login' first.")
	}
}

func assembleLocalResourceList(filter *openshift.ResourceFilter, compareOptions *cli.CompareOptions) *openshift.ResourceList {
	list := &openshift.ResourceList{Filter: filter}

	// read files in folders and assemble lists for kinds
	for i, templateDir := range compareOptions.TemplateDirs {
		files, err := ioutil.ReadDir(templateDir)
		if err != nil {
			log.Fatal(err)
		}
		filePattern := ".*\\.ya?ml$"
		for _, file := range files {
			matched, _ := regexp.MatchString(filePattern, file.Name())
			if !matched {
				continue
			}
			cli.DebugMsg("Reading", file.Name())
			processedOut, err := openshift.ProcessTemplate(templateDir, file.Name(), compareOptions.ParamDirs[i], compareOptions)
			if err != nil {
				log.Fatalln("Could not process", file.Name(), "template:", err)
			}
			processedConfig := openshift.NewConfigFromList(processedOut)
			list.AppendItems(processedConfig)
		}
	}

	return list
}

func assembleRemoteResourceList(filter *openshift.ResourceFilter, compareOptions *cli.CompareOptions) *openshift.ResourceList {
	list := &openshift.ResourceList{Filter: filter}

	exportedOut, err := openshift.ExportResources(filter, compareOptions)
	if err != nil {
		log.Fatalln("Could not export", filter.String(), " resources.")
	}
	exportedConfig := openshift.NewConfigFromList(exportedOut)
	list.AppendItems(exportedConfig)

	return list
}

func export(filter *openshift.ResourceFilter, exportOptions *cli.ExportOptions) {
	var templateName string
	if len(filter.Name) > 0 {
		templateName = strings.Replace(filter.Name, "/", "-", -1)
	} else if len(filter.Label) > 0 {
		labelParts := strings.Split(filter.Label, "=")
		templateName = labelParts[1]
	} else if len(filter.Kinds) > 0 {
		templateName = strings.ToLower(strings.Join(filter.Kinds, "-"))
	} else {
		templateName = "all"
	}

	out, err := openshift.ExportAsTemplate(filter, templateName, exportOptions)
	if err != nil {
		log.Fatalln("Could not export", filter.String(), "resources as template.")
	}
	if len(out) == 0 {
		return
	}

	config := openshift.NewConfigFromTemplate(out)

	b, _ := yaml.Marshal(config.Processed)
	fmt.Println(string(b))
}

func compare(remoteResourceList *openshift.ResourceList, localResourceList *openshift.ResourceList, upsertOnly bool) *openshift.Changeset {
	changeset := openshift.NewChangeset(remoteResourceList, localResourceList, upsertOnly)

	for _, change := range changeset.Noop {
		fmt.Printf("* %s is in sync\n", change.ItemName())
	}

	for _, change := range changeset.Delete {
		cli.PrintRedf("- %s to be deleted\n", change.ItemName())
		cli.ShowDiff(change.CurrentState, change.DesiredState)
	}

	for _, change := range changeset.Create {
		cli.PrintGreenf("+ %s to be created\n", change.ItemName())
		cli.ShowDiff(change.CurrentState, change.DesiredState)
	}

	for _, change := range changeset.Update {
		cli.PrintYellowf("~ %s to be updated\n", change.ItemName())
		cli.ShowDiff(change.CurrentState, change.DesiredState)
	}

	return changeset
}
