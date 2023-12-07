/*
Copyright © 2023 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"fmt"
	"magician/cloudbuild"
	"magician/github"
	"os"

	"github.com/spf13/cobra"
)

type mcGithub interface {
	GetPullRequestAuthor(prNumber string) (string, error)
	GetUserType(user string) github.UserType
	GetPullRequestRequestedReviewer(prNumber string) (string, error)
	GetPullRequestPreviousAssignedReviewers(prNumber string) ([]string, error)
	RequestPullRequestReviewer(prNumber string, reviewer string) error
	PostComment(prNumber string, comment string) error
	AddLabel(prNumber string, label string) error
	PostBuildStatus(prNumber string, title string, state string, targetUrl string, commitSha string) error
}

type mcCloudbuild interface {
	ApproveCommunityChecker(prNumber, commitSha string) error
	GetAwaitingApprovalBuildLink(prNumber, commitSha string) (string, error)
	TriggerMMPresubmitRuns(commitSha string, substitutions map[string]string) error
}

// membershipCheckerCmd represents the membershipChecker command
var membershipCheckerCmd = &cobra.Command{
	Use:   "request-service-reviewers PR_NUMBER",
	Short: "Assigns reviewers based on the PR's service labels.",
	Long: `This command requests (or re-requests) review based on the PR's service labels.

	If a PR has more than 3 service labels, the command will not do anything.
	`,
	Args: cobra.ExactArgs(6),
	Run: func(cmd *cobra.Command, args []string) {
		prNumber := args[0]
		fmt.Println("PR Number: ", prNumber)

		gh := github.NewGithubService()
		execRequestServiceReviewers(prNumber, gh)
	},
}

func execRequestServiceReviewers(prNumber string, gh mcGithub) {
	substitutions := map[string]string{
		"BRANCH_NAME":    branchName,
		"_PR_NUMBER":     prNumber,
		"_HEAD_REPO_URL": headRepoUrl,
		"_HEAD_BRANCH":   headBranch,
		"_BASE_BRANCH":   baseBranch,
	}

	author, err := gh.GetPullRequestAuthor(prNumber)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	authorUserType := gh.GetUserType(author)
	trusted := authorUserType == github.CoreContributorUserType || authorUserType == github.GooglerUserType

	if authorUserType != github.CoreContributorUserType {
		fmt.Println("Not core contributor - assigning reviewer")

		firstRequestedReviewer, err := gh.GetPullRequestRequestedReviewer(prNumber)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}

		previouslyInvolvedReviewers, err := gh.GetPullRequestPreviousAssignedReviewers(prNumber)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}

		reviewersToRequest, newPrimaryReviewer := github.ChooseCoreReviewers(firstRequestedReviewer, previouslyInvolvedReviewers)

		for _, reviewer := range reviewersToRequest {
			err = gh.RequestPullRequestReviewer(prNumber, reviewer)
			if err != nil {
				fmt.Println(err)
				os.Exit(1)
			}
		}

		if newPrimaryReviewer != "" {
			comment := github.FormatReviewerComment(newPrimaryReviewer, authorUserType, trusted)
			err = gh.PostComment(prNumber, comment)
			if err != nil {
				fmt.Println(err)
				os.Exit(1)
			}
		}
	}

	// auto_run(contributor-membership-checker) will be run on every commit or /gcbrun:
	// only triggers builds for trusted users
	if trusted {
		err = cb.TriggerMMPresubmitRuns(commitSha, substitutions)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
	}

	// in contributor-membership-checker job:
	// 1. auto approve community-checker run for trusted users
	// 2. add awaiting-approval label to external contributor PRs
	if trusted {
		err = cb.ApproveCommunityChecker(prNumber, commitSha)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
	} else {
		gh.AddLabel(prNumber, "awaiting-approval")
		targetUrl, err := cb.GetAwaitingApprovalBuildLink(prNumber, commitSha)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}

		err = gh.PostBuildStatus(prNumber, "Approve Build", "success", targetUrl, commitSha)
		if err != nil {
			fmt.Println(err)
			os.Exit(1)
		}
	}
}

func init() {
	rootCmd.AddCommand(membershipCheckerCmd)
}
