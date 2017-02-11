package cmd

import (
	"flag"
	"fmt"

	"github.com/driusan/go-git/git"
)

func DiffFiles(c *git.Client, args []string) error {
	flags := flag.NewFlagSet("diff-tree", flag.ExitOnError)
	options := git.DiffFilesOptions{}

	patch := flags.Bool("index", false, "Generate patch")
	p := flags.Bool("p", false, "Alias for --patch")
	u := flags.Bool("u", false, "Alias for --patch")

	nopatch := flags.Bool("no-patch", false, "Suppress patch generation")
	s := flags.Bool("s", false, "Alias of --no-patch")

	//unified := flags.Int("unified", 3, "Generate <n> lines of context")
	//U := flags.Int("U", 3, "Alias of --unified")
	flags.BoolVar(&options.Raw, "raw", true, "Generate the diff in raw format")
	flags.BoolVar(&options.Recurse, "r", false, "Recurse into subtrees")

	flags.Parse(args)
	args = flags.Args()

	if *patch || *p || *u {
		options.Patch = true
	}
	if *nopatch || *s {
		options.Patch = false
	}

	/*
		if unified != nil && U != nil {
			return fmt.Errorf("Can not specify both --unified and -U")
		} else if unified != nil {
			options.NumContextLines = *unified
		} else if U != nil {
			options.NumContextLines = *U
		} else {
	*/
	options.NumContextLines = 3

	diffs, err := git.DiffFiles(c, &options, args)
	for _, diff := range diffs {
		fmt.Printf("%v\n", diff)
	}
	return err
}
