package git

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
)

// CheckoutOptions represents the options that may be passed to
// "git checkout"
type CheckoutOptions struct {
	// Not implemented
	Quiet bool
	// Not implemented
	Progress bool
	// Not implemented
	Force bool

	// Check out the named stage for unnamed paths.
	// Stage2 is equivalent to --ours, Stage3 to --theirs
	// Not implemented
	Stage Stage

	// Not implemented
	Branch string // -b
	// Not implemented
	ForceBranch bool // use branch as -B
	// Not implemented
	OrphanBranch bool // use branch as --orphan

	// Not implemented
	Track string
	// Not implemented
	CreateReflog bool // -l

	// Not implemented
	Detach bool

	// Not implemented
	IgnoreSkipWorktreeBits bool
	// Not implemented
	Merge bool

	// Not implemented.
	ConflictStyle string

	Patch bool

	// Not implemented
	IgnoreOtherWorktrees bool
}

// Implements the "git checkout" subcommand of git. Variations in the man-page
// are:
//
//     git checkout [-q] [-f] [-m] [<branch>]
//     git checkout [-q] [-f] [-m] --detach [<branch>]
//     git checkout [-q] [-f] [-m] [--detach] <commit>
//     git checkout [-q] [-f] [-m] [[-b|-B|--orphan] <new_branch>] [<start_point>]
//     git checkout [-f|--ours|--theirs|-m|--conflict=<style>] [<tree-ish>] [--] <paths>...
//     git checkout [-p|--patch] [<tree-ish>] [--] [<paths>...]
//
// This will just check the options and call the appropriate variation. You
// can avoid the overhead by calling the proper variation directly.
//
// "thing" is the thing that the user entered on the command line to be checked out. It
// might be a branch, a commit, or a treeish, depending on the variation above.
func Checkout(c *Client, opts CheckoutOptions, thing string, files []File) error {
	if thing == "" {
		thing = "HEAD"
	}

	if opts.Patch {
		diffs, err := DiffFiles(c, DiffFilesOptions{}, files)
		if err != nil {
			return err
		}
		var patchbuf bytes.Buffer
		if err := GeneratePatch(c, DiffCommonOptions{Patch: true}, diffs, &patchbuf); err != nil {
			return err
		}
		hunks, err := splitPatch(patchbuf.String(), false)
		if err != nil {
			return err
		}
		hunks, err = filterHunks("discard this hunk from the work tree", hunks)
		if err == userAborted {
			return nil
		} else if err != nil {
			return err
		}

		patch, err := ioutil.TempFile("", "checkoutpatch")
		if err != nil {
			return err
		}
		defer os.Remove(patch.Name())
		recombinePatch(patch, hunks)

		return Apply(c, ApplyOptions{Reverse: true}, []File{File(patch.Name())})
	}

	if len(files) == 0 {
		cmt, err := RevParseCommitish(c, &RevParseOptions{}, thing)
		if err != nil {
			return err
		}
		return CheckoutCommit(c, opts, cmt)
	}

	b, err := RevParseTreeish(c, &RevParseOptions{}, thing)
	if err != nil {
		return err
	}
	return CheckoutFiles(c, opts, b, files)
}

// Implements the "git checkout" subcommand of git for variations:
//     git checkout [-q] [-f] [-m] [<branch>]
//     git checkout [-q] [-f] [-m] --detach [<branch>]
//     git checkout [-q] [-f] [-m] [--detach] <commit>
//     git checkout [-q] [-f] [-m] [[-b|-B|--orphan] <new_branch>] [<start_point>]
func CheckoutCommit(c *Client, opts CheckoutOptions, commit Commitish) error {
	// RefSpec for new branch with -b/-B variety
	var newRefspec RefSpec
	if opts.Branch != "" {
		// Handle the -b/-B variety.
		// commit is the startpoint in the last variation, otherwise
		// Checkout() already set it to the commit of "HEAD"
		newRefspec = RefSpec("refs/heads/" + opts.Branch)
		refspecfile := newRefspec.File(c)
		if refspecfile.Exists() && !opts.ForceBranch {
			return fmt.Errorf("Branch %s already exists.", opts.Branch)
		}
	}
	// Get the original HEAD for the reflog
	head, err := SymbolicRefGet(c, SymbolicRefOptions{}, "HEAD")
	if err != nil && err != DetachedHead {
		return err
	}

	// Convert from Commitish to Treeish for ReadTree
	cid, err := commit.CommitID(c)
	if err != nil {
		return err
	}

	readtreeopts := ReadTreeOptions{Update: true, Merge: true}
	if opts.Force {
		readtreeopts.Merge = false
		readtreeopts.Reset = true
	}
	if _, err := ReadTree(c, readtreeopts, cid); err != nil {
		return err
	}

	var origB string
	// Get the original HEAD branchname for the reflog
	origB = Branch(head).BranchName()
	if branch := Branch(head).BranchName(); branch != "" {
		origB = branch
	} else {
		if h, err := head.CommitID(c); err == nil {
			origB = h.String()
		}
	}

	if b, ok := commit.(Branch); ok && !opts.Detach {
		// We're checking out a branch, first read the new tree, and
		// then update the SymbolicRef for HEAD, if that succeeds.
		refmsg := fmt.Sprintf("checkout: moving from %s to %s (dgit)", origB, b.BranchName())
		return SymbolicRefUpdate(c, SymbolicRefOptions{}, "HEAD", RefSpec(b), refmsg)
	}
	refmsg := fmt.Sprintf("checkout: moving from %s to %s (dgit)", origB, cid)
	if err := UpdateRef(c, UpdateRefOptions{NoDeref: true, OldValue: head}, "HEAD", cid, refmsg); err != nil {
		return err
	}
	return nil
}

// Implements "git checkout" subcommand of git for variations:
//     git checkout [-f|--ours|--theirs|-m|--conflict=<style>] [<tree-ish>] [--] <paths>...
//     git checkout [-p|--patch] [<tree-ish>] [--] [<paths>...]
func CheckoutFiles(c *Client, opts CheckoutOptions, tree Treeish, files []File) error {
	// If files were specified, we don't want ReadTree to update the workdir,
	// because we only want to (force) update the specified files.
	//
	// If they weren't, we want to checkout a treeish, so let ReadTree update
	// the workdir so that we don't lose any changes.
	updateAll := (len(files) == 0)
	i, err := ReadTree(c, ReadTreeOptions{Update: updateAll, Merge: updateAll}, tree)
	if err != nil {
		return err
	}
	// ReadTree wrote the index to disk, but since we already have a copy in
	// memory we use the Uncommited variation.
	return CheckoutIndexUncommited(c, i, CheckoutIndexOptions{Force: true, UpdateStat: true}, files)
}
