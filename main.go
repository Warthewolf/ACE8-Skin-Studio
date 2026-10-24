package main

import "os"

func main() {
	if len(os.Args) > 1 && os.Args[1] == "cli" {
		os.Exit(runCLI(os.Args[2:]))
	}
	if len(os.Args) > 1 && os.Args[1] == pickHelperArg {
		os.Exit(runPickHelper()) // the short-lived copy that shows a file chooser
	}
	os.Exit(runApp(os.Args[1:]))
}
