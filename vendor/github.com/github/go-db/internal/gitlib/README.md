# Package gitlib

### [godocs](https://godoc.githubapp.com/github.com/github/go-db/gitlib)

This implements a helper library meant to simplify the process of creating a branch, making changes and opening a pull request.

## Example

```golang
package main

import (
  "fmt"
  "github.com/github/db-scripts/gitlib"
	"github.com/google/go-github/github"
  "golang.org/x/oauth2"
)

func main() {
  // Initialize the github API client with an access token.  Hubot is a common
  // token to use & the group Robots needs admin permissions to the target
  // repository for these actions to work.
  tokenSource := oauth2.StaticTokenSource(
    &oauth2.Token{AccessToken: token},
  )
  client := github.NewClient(
    oauth2.NewClient(context.Background(), tokenSource))

  var repoName = "my-repo"

  // initialize the helper object
  helper := gitlib.NewHelper(client)

  // Check permissions
  if !helper.HubotHasAdmin(repoName) {
    log.Fatal("Hubot doesn't have admin permissions to the repo")
  }

  // create a branch from master in the "repo-name" repo
  branch, _ := helper.NewBranchFromMaster(repoName,
    gitlib.GenerateUniqueBranchName("mybranch"))

  // Add a new file and commit the changes to the branch
  branch.AddFiles([]gitlib.FileContents{
    gitlib.FileContents{
      Path: "docs/README.md",
      Mode: "100644",
      Contents: "# This is a markdown file!",
    },
  })
  branch.CommitChanges("Add README.md in docs")

  // Open a new PR and report the URL
  pr, _ := branch.CreatePR("Pull Request Title", "body and details here")
  fmt.Printf("The PR URL is: %s\n", pr.GetHTMLURL())
}
```
