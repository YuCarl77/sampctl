package rook

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/google/go-github/github"
	"github.com/pkg/errors"
	"gopkg.in/AlecAivazis/survey.v1"
	"gopkg.in/src-d/go-git.v4"
	"gopkg.in/src-d/go-git.v4/config"
	"gopkg.in/src-d/go-git.v4/plumbing"
	"gopkg.in/src-d/go-git.v4/plumbing/transport"

	"github.com/Southclaws/sampctl/print"
	"github.com/Southclaws/sampctl/types"
	"github.com/Southclaws/sampctl/versioning"
)

// Release is an interactive release tool for package versioning
func Release(ctx context.Context, gh *github.Client, auth transport.AuthMethod, pkg types.Package) (err error) {
	repo, err := git.PlainOpen(pkg.Local)
	if err != nil {
		return errors.Wrap(err, "failed to read package as git repository")
	}

	head, err := repo.Head()
	if err != nil {
		return errors.Wrap(err, "failed to get repo HEAD reference")
	}

	tags, err := versioning.GetRepoSemverTags(repo)
	if err != nil {
		return errors.Wrap(err, "failed to get semver tags")
	}
	sort.Sort(sort.Reverse(tags))

	var questions []*survey.Question
	var answers struct {
		Version      string
		Distribution bool
		GitHub       bool
	}

	if len(tags) == 0 {
		questions = []*survey.Question{
			{
				Name: "Version",
				Prompt: &survey.Select{
					Message: "New Project Version",
					Options: []string{
						"0.0.1: Unstable prototype",
						"0.1.0: Stable prototype but subject to change",
						"1.0.0: Stable release, API won't change",
					},
				},
				Validate: survey.Required,
			},
		}
	} else {
		var latest versioning.VersionedTag = tags[0]

		print.Info("Latest version:", latest.Tag)

		bumpPatch := latest.Tag.IncPatch()
		bumpMinor := latest.Tag.IncMinor()
		bumpMajor := latest.Tag.IncMajor()

		questions = []*survey.Question{
			{
				Name: "Version",
				Prompt: &survey.Select{
					Message: "Select Version Bump",
					Options: []string{
						fmt.Sprintf("%s: I made backwards-compatible bug fixes", bumpPatch.String()),
						fmt.Sprintf("%s: I added functionality in a backwards-compatible manner", bumpMinor.String()),
						fmt.Sprintf("%s: I made incompatible API changes", bumpMajor.String()),
					},
				},
				Validate: survey.Required,
			},
		}
	}

	// questions = append(questions, &survey.Question{
	// 	Name: "Distribution",
	// 	Prompt: &survey.Confirm{
	// 		Message: "Create Distribution Release?",
	// 		Default: false,
	// 	},
	// })

	questions = append(questions, &survey.Question{
		Name: "GitHub",
		Prompt: &survey.Confirm{
			Message: "Create GitHub Release? (requires `github_token` token to be set in `~/.samp/config.json`)",
			Default: false,
		},
	})

	err = survey.Ask(questions, &answers)
	if err != nil {
		return errors.Wrap(err, "failed to open wizard")
	}
	newVersion := strings.Split(answers.Version, ":")[0]

	print.Info("New version:", newVersion)

	ref := plumbing.ReferenceName("refs/tags/" + newVersion)
	hash := plumbing.NewHashReference(ref, head.Hash())
	err = repo.Storer.SetReference(hash)

	if answers.GitHub {
		print.Info("Pushing", newVersion, "to remote")
		err = repo.Push(&git.PushOptions{
			RefSpecs: []config.RefSpec{config.RefSpec("refs/tags/*:refs/tags/*")},
			Auth:     auth,
		})
		if err != nil {
			if err.Error() == "authentication required" {
				print.Erro("Please set `github_token` to a GitHub API token in `~/.samp/config.json`")
			} else if err.Error() == "already up-to-date" {
				err = nil
			}
			return errors.Wrap(err, "failed to push")
		}

		// todo: generate changelog

		print.Info("Creating release for", newVersion)
		release, _, err := gh.Repositories.CreateRelease(ctx, pkg.User, pkg.Repo, &github.RepositoryRelease{
			TagName: &newVersion,
			Name:    &newVersion,
			Draft:   &[]bool{true}[0],
		})
		if err != nil {
			return errors.Wrap(err, "failed to create release")
		}

		print.Info("Released at:", fmt.Sprintf("https://github.com/%s/%s/releases", pkg.User, pkg.Repo))
	}

	if answers.Distribution {
		// todo: zip the package in a `pawno/include` style
		// possibly include dependencies too
	}

	return
}
