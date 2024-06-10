package getazure

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/briandowns/spinner"
	"github.com/microsoft/azure-devops-go-api/azuredevops"
	"github.com/microsoft/azure-devops-go-api/azuredevops/core"
	"github.com/microsoft/azure-devops-go-api/azuredevops/git"
)

type ProjectBranch struct {
	Org         string
	ProjectKey  string
	RepoSlug    string
	MainBranch  string
	LargestSize int64
}

/*type ExclusionList struct {
	Repos map[string]bool `json:"repos"`
}*/

type AnalysisResult struct {
	NumRepositories int
	ProjectBranches []ProjectBranch
}

type ExclusionList struct {
	Projects map[string]bool
	Repos    map[string]bool
}
type SummaryStats struct {
	LargestRepo       string
	LargestRepoBranch string
	NbRepos           int
	EmptyRepo         int
	TotalExclude      int
	TotalArchiv       int
	TotalBranches     int
}

type AnalyzeProject struct {
	Project       core.TeamProjectReference
	AzureClient   core.Client
	Context       context.Context
	ExclusionList *ExclusionList
	Spin1         *spinner.Spinner
	Org           string
}

type ParamsProjectAzure struct {
	Client         core.Client
	Context        context.Context
	Projects       []core.TeamProjectReference
	URL            string
	AccessToken    string
	ApiURL         string
	Organization   string
	Exclusionlist  *ExclusionList
	Excludeproject int
	Spin           *spinner.Spinner
	Period         int
	Stats          bool
	DefaultB       bool
	SingleRepos    string
	SingleBranch   string
}

// RepositoryMap represents a map of repositories to ignore
type ExclusionRepos map[string]bool

const PrefixMsg = "Get Project(s)..."
const MessageErro1 = "/\n❌ Failed to list projects for organization %s: %v\n"
const MessageErro2 = "/\n❌ Failed to list project for organization %s: %v\n"
const Message1 = "\t✅ The number of %s found is: %d\n"
const Message2 = "\t   Analysis top branch(es) in project <%s> ..."
const Message3 = "\r\t\t✅ %d Project: %s - Number of branches: %d - largest Branch: %s \n"
const Message4 = "Project(s)"

func loadExclusionList(filename string) (*ExclusionList, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	exclusionList := &ExclusionList{
		Projects: make(map[string]bool),
		Repos:    make(map[string]bool),
	}

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.Split(line, "/")
		if len(parts) == 1 {
			// Exclusion de projet
			exclusionList.Projects[parts[0]] = true
		} else if len(parts) == 2 {
			// Exclusion de répertoire
			exclusionList.Repos[line] = true
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return exclusionList, nil
}

func loadExclusionFileOrCreateNew(exclusionFile string) (*ExclusionList, error) {
	if exclusionFile == "0" {
		return &ExclusionList{
			Projects: make(map[string]bool),
			Repos:    make(map[string]bool),
		}, nil
	}
	return loadExclusionList(exclusionFile)
}

func isRepoExcluded(exclusionList *ExclusionList, projectKey, repoKey string) bool {
	_, repoExcluded := exclusionList.Repos[projectKey+"/"+repoKey]
	return repoExcluded
}

// Fonction pour vérifier si un projet est exclu
func isProjectExcluded(exclusionList *ExclusionList, projectKey string) bool {
	_, projectExcluded := exclusionList.Projects[projectKey]
	return projectExcluded
}

func isRepoEmpty(ctx context.Context, gitClient git.Client, projectID string, repoID string) (bool, error) {
	path := "/"
	items, err := gitClient.GetItems(ctx, git.GetItemsArgs{
		RepositoryId:   &repoID,
		Project:        &projectID,
		ScopePath:      &path,
		RecursionLevel: &git.VersionControlRecursionTypeValues.None,
	})
	if err != nil {
		return false, err
	}

	return len(*items) == 0, nil
}

func getAllProjects(ctx context.Context, coreClient core.Client, exclusionList *ExclusionList) ([]core.TeamProjectReference, int, error) {
	var allProjects []core.TeamProjectReference
	var excludedCount int
	var continuationToken string

	for {
		// Get the current projects page
		responseValue, err := coreClient.GetProjects(ctx, core.GetProjectsArgs{
			ContinuationToken: &continuationToken,
		})
		if err != nil {
			return nil, 0, err
		}

		for _, project := range responseValue.Value {
			if isProjectExcluded(exclusionList, *project.Name) {
				excludedCount++
				continue
			}

			allProjects = append(allProjects, project)

		}

		// Check if there is a continuation token for the next page
		if responseValue.ContinuationToken == "" {
			break
		}

		// Update the continuation token
		continuationToken = responseValue.ContinuationToken
	}

	return allProjects, excludedCount, nil
}

func getProjectByName(ctx context.Context, coreClient core.Client, projectName string, exclusionList *ExclusionList) ([]core.TeamProjectReference, int, error) {

	var excludedCount int

	if isProjectExcluded(exclusionList, projectName) {
		excludedCount++
		errmessage := fmt.Sprintf(" - Skipping analysis for Project %s , it is excluded", projectName)
		err := fmt.Errorf(errmessage)
		return nil, excludedCount, err
	}

	project, err := coreClient.GetProject(ctx, core.GetProjectArgs{
		ProjectId: &projectName,
	})
	if err != nil {
		return nil, 0, err
	}

	// Create a core.TeamProjectReference from core.TeamProject
	projectReference := core.TeamProjectReference{
		Id:          project.Id,
		Name:        project.Name,
		Description: project.Description,
		Url:         project.Url,
		State:       project.State,
		Revision:    project.Revision,
	}

	return []core.TeamProjectReference{projectReference}, excludedCount, nil
}

func GetRepoAzureList(platformConfig map[string]interface{}, exclusionFile string) ([]ProjectBranch, error) {

	var importantBranches []ProjectBranch
	var totalExclude, totalArchiv, emptyRepo, TotalBranches, nbRepos int
	var totalSize int64
	var largestRepoBranch, largesRepo string
	//	var emptyRepos, archivedRepos int
	//	var TotalBranches int = 0 // Counter Number of Branches on All Repositories
	var exclusionList *ExclusionList
	var err error

	//var totalExclude, totalArchiv, emptyRepo, TotalBranches, exludedprojects int
	//var nbRepos int

	//	var totalSize int

	//	excludedProjects := 0
	//	result := AnalysisResult{}

	// Calculating the period
	//	until := time.Now()
	//	since := until.AddDate(0, int(platformConfig["Period"].(float64)), 0)
	ApiURL := platformConfig["Url"].(string) + platformConfig["Organization"].(string)

	fmt.Print("\n🔎 Analysis of devops platform objects ...\n")

	spin := spinner.New(spinner.CharSets[35], 100*time.Millisecond)
	spin.Prefix = PrefixMsg
	spin.Color("green", "bold")
	spin.Start()

	exclusionList, err = loadExclusionFileOrCreateNew(exclusionFile)
	if err != nil {
		fmt.Printf("\n❌ Error Read Exclusion File <%s>: %v", exclusionFile, err)
		spin.Stop()
		return nil, err
	}

	// Create a connection to your organization
	connection := azuredevops.NewPatConnection(ApiURL, platformConfig["AccessToken"].(string))
	ctx := context.Background()

	// Create a client to interact with the Core area
	coreClient, err := core.NewClient(ctx, connection)
	if err != nil {
		log.Fatal(err)
	}

	gitClient, err := git.NewClient(ctx, connection)
	if err != nil {
		log.Fatalf("Erreur lors de la création du client Git: %v", err)
	}

	//	cpt := 1

	/* --------------------- Analysis all projects with a default branche  ---------------------  */
	if platformConfig["Project"].(string) == "" {

		// Get All Project
		projects, exludedprojects, err := getAllProjects(ctx, coreClient, exclusionList)

		if err != nil {
			spin.Stop()
			log.Fatalf(MessageErro1, platformConfig["Organization"].(string), err)
		}
		spin.Stop()
		spin1 := spinner.New(spinner.CharSets[35], 100*time.Millisecond)
		spin1.Color("green", "bold")

		fmt.Printf(Message1, Message4, len(projects)+exludedprojects)

		// Set Parmams
		params := getCommonParams(ctx, coreClient, platformConfig, projects, exclusionList, exludedprojects, spin, ApiURL)
		// Analyse Get important Branch
		importantBranches, emptyRepo, nbRepos, TotalBranches, totalExclude, totalArchiv, err = getRepoAnalyse(params, gitClient)
		if err != nil {
			spin.Stop()
			return nil, err
		}

	} else {
		projects, exludedprojects, err := getProjectByName(ctx, coreClient, platformConfig["Project"].(string), exclusionList)
		if err != nil {
			spin.Stop()
			log.Fatalf(MessageErro2, platformConfig["Organization"].(string), err)
		}

		spin.Stop()
		spin1 := spinner.New(spinner.CharSets[35], 100*time.Millisecond)
		spin1.Color("green", "bold")

		fmt.Printf(Message1, Message4, 1+exludedprojects)

		// Set Parmams
		params := getCommonParams(ctx, coreClient, platformConfig, projects, exclusionList, exludedprojects, spin, ApiURL)
		// Analyse Get important Branch
		importantBranches, emptyRepo, nbRepos, TotalBranches, totalExclude, totalArchiv, err = getRepoAnalyse(params, gitClient)
		if err != nil {
			spin.Stop()
			return nil, err
		}
	}

	largestRepoBranch, largesRepo = findLargestRepository(importantBranches, &totalSize)

	result := AnalysisResult{
		NumRepositories: nbRepos,
		ProjectBranches: importantBranches,
	}
	if err := SaveResult(result); err != nil {
		fmt.Println("❌ Error Save Result of Analysis :", err)
		os.Exit(1)
	}

	stats := SummaryStats{
		LargestRepo:       largesRepo,
		LargestRepoBranch: largestRepoBranch,
		NbRepos:           nbRepos,
		EmptyRepo:         emptyRepo,
		TotalExclude:      totalExclude,
		TotalArchiv:       totalArchiv,
		TotalBranches:     TotalBranches,
	}

	printSummary(platformConfig["Organization"].(string), stats)
	os.Exit(1)
	return importantBranches, nil
}

func GetRepoAzureList1(platformConfig map[string]interface{}, exclusionFile string) ([]ProjectBranch, error) {

	var importantBranches []ProjectBranch
	var totalExclude, totalArchiv, emptyRepo, TotalBranches, nbRepos int
	var totalSize int64
	var largestRepoBranch, largesRepo string
	//	var emptyRepos, archivedRepos int
	//	var TotalBranches int = 0 // Counter Number of Branches on All Repositories
	var exclusionList *ExclusionList
	var err error

	//var totalExclude, totalArchiv, emptyRepo, TotalBranches, exludedprojects int
	//var nbRepos int

	//	var totalSize int

	//	excludedProjects := 0
	//	result := AnalysisResult{}

	// Calculating the period
	//	until := time.Now()
	//	since := until.AddDate(0, int(platformConfig["Period"].(float64)), 0)
	ApiURL := platformConfig["Url"].(string) + platformConfig["Organization"].(string)

	fmt.Print("\n🔎 Analysis of devops platform objects ...\n")

	spin := spinner.New(spinner.CharSets[35], 100*time.Millisecond)
	spin.Prefix = PrefixMsg
	spin.Color("green", "bold")
	spin.Start()

	exclusionList, err = loadExclusionFileOrCreateNew(exclusionFile)
	if err != nil {
		fmt.Printf("\n❌ Error Read Exclusion File <%s>: %v", exclusionFile, err)
		spin.Stop()
		return nil, err
	}

	// Create a connection to your organization
	connection := azuredevops.NewPatConnection(ApiURL, platformConfig["AccessToken"].(string))
	ctx := context.Background()

	// Create a client to interact with the Core area
	coreClient, err := core.NewClient(ctx, connection)
	if err != nil {
		log.Fatal(err)
	}

	gitClient, err := git.NewClient(ctx, connection)
	if err != nil {
		log.Fatalf("Erreur lors de la création du client Git: %v", err)
	}

	if platformConfig["DefaultBranch"].(bool) {
		//	cpt := 1

		/* --------------------- Analysis all projects with a default branche  ---------------------  */
		if platformConfig["Project"].(string) == "" {

			// Get All Project
			projects, exludedprojects, err := getAllProjects(ctx, coreClient, exclusionList)

			if err != nil {
				spin.Stop()
				log.Fatalf(MessageErro1, platformConfig["Organization"].(string), err)
			}
			spin.Stop()
			spin1 := spinner.New(spinner.CharSets[35], 100*time.Millisecond)
			spin1.Color("green", "bold")

			fmt.Printf(Message1, Message4, len(projects)+exludedprojects)

			// Set Parmams
			params := getCommonParams(ctx, coreClient, platformConfig, projects, exclusionList, exludedprojects, spin, ApiURL)
			// Analyse Get important Branch
			importantBranches, emptyRepo, nbRepos, TotalBranches, totalExclude, totalArchiv, err = getRepoAnalyse(params, gitClient)
			if err != nil {
				spin.Stop()
				return nil, err
			}

		} else {
			projects, exludedprojects, err := getProjectByName(ctx, coreClient, platformConfig["Project"].(string), exclusionList)
			if err != nil {
				spin.Stop()
				log.Fatalf(MessageErro2, platformConfig["Organization"].(string), err)
			}

			spin.Stop()
			spin1 := spinner.New(spinner.CharSets[35], 100*time.Millisecond)
			spin1.Color("green", "bold")

			fmt.Printf(Message1, Message4, 1+exludedprojects)

			// Set Parmams
			params := getCommonParams(ctx, coreClient, platformConfig, projects, exclusionList, exludedprojects, spin, ApiURL)
			// Analyse Get important Branch
			importantBranches, emptyRepo, nbRepos, TotalBranches, totalExclude, totalArchiv, err = getRepoAnalyse(params, gitClient)
			if err != nil {
				spin.Stop()
				return nil, err
			}
		}
	}

	largestRepoBranch, largesRepo = findLargestRepository(importantBranches, &totalSize)

	result := AnalysisResult{
		NumRepositories: nbRepos,
		ProjectBranches: importantBranches,
	}
	if err := SaveResult(result); err != nil {
		fmt.Println("❌ Error Save Result of Analysis :", err)
		os.Exit(1)
	}

	stats := SummaryStats{
		LargestRepo:       largesRepo,
		LargestRepoBranch: largestRepoBranch,
		NbRepos:           nbRepos,
		EmptyRepo:         emptyRepo,
		TotalExclude:      totalExclude,
		TotalArchiv:       totalArchiv,
		TotalBranches:     TotalBranches,
	}

	printSummary(platformConfig["Organization"].(string), stats)
	os.Exit(1)
	return importantBranches, nil
}

func getCommonParams(ctx context.Context, client core.Client, platformConfig map[string]interface{}, project []core.TeamProjectReference, exclusionList *ExclusionList, excludeproject int, spin *spinner.Spinner, apiURL string) ParamsProjectAzure {
	return ParamsProjectAzure{
		Client:   client,
		Context:  ctx,
		Projects: project,

		URL:            platformConfig["Url"].(string),
		AccessToken:    platformConfig["AccessToken"].(string),
		ApiURL:         apiURL,
		Organization:   platformConfig["Organization"].(string),
		Exclusionlist:  exclusionList,
		Excludeproject: excludeproject,
		Spin:           spin,
		Period:         int(platformConfig["Period"].(float64)),
		Stats:          platformConfig["Stats"].(bool),
		DefaultB:       platformConfig["DefaultBranch"].(bool),
		SingleRepos:    platformConfig["Repos"].(string),
		SingleBranch:   platformConfig["Branch"].(string),
	}
}

func findLargestRepository(importantBranches []ProjectBranch, totalSize *int64) (string, string) {

	var largestRepoBranch, largesRepo string
	var largestRepoSize int64 = 0

	for _, branch := range importantBranches {
		if branch.LargestSize > int64(largestRepoSize) {
			largestRepoSize = branch.LargestSize
			largestRepoBranch = branch.MainBranch
			largesRepo = branch.RepoSlug

		}
		*totalSize += branch.LargestSize
	}
	return largestRepoBranch, largesRepo
}

func printSummary(Org string, stats SummaryStats) {
	fmt.Printf("\n✅ The largest Repository is <%s> in the organization <%s> with the branch <%s> \n", stats.LargestRepo, Org, stats.LargestRepoBranch)
	fmt.Printf("\r✅ Total Repositories that will be analyzed: %d - Find empty : %d - Excluded : %d - Archived : %d\n", stats.NbRepos-stats.EmptyRepo-stats.TotalExclude-stats.TotalArchiv, stats.EmptyRepo, stats.TotalExclude, stats.TotalArchiv)
	fmt.Printf("\r✅ Total Branches that will be analyzed: %d\n", stats.TotalBranches)
}

func getRepoAnalyse(params ParamsProjectAzure, gitClient git.Client) ([]ProjectBranch, int, int, int, int, int, error) {

	var emptyRepos = 0
	var totalexclude = 0
	var importantBranches []ProjectBranch
	var NBRrepo, TotalBranches int
	var messageF = ""
	NBRrepos := 0
	cptarchiv := 0

	cpt := 1

	message4 := "Repo(s)"

	spin1 := spinner.New(spinner.CharSets[35], 100*time.Millisecond)
	spin1.Prefix = PrefixMsg
	spin1.Color("green", "bold")

	params.Spin.Start()
	if params.Excludeproject > 0 {
		messageF = fmt.Sprintf("\t✅ The number of project(s) to analyze is %d - Excluded : %d\n", len(params.Projects), params.Excludeproject)
	} else {
		messageF = fmt.Sprintf("\t✅ The number of project(s) to analyze is %d\n", len(params.Projects))
	}
	params.Spin.FinalMSG = messageF
	params.Spin.Stop()

	// Get Repository in each Project
	for _, project := range params.Projects {

		//	if !isProjectExcluded(params.Exclusionlist, *project.Name) {
		fmt.Printf("\n\t🟢  Analyse Projet: %s \n", *project.Name)

		emptyOrArchivedCount, emptyRepos, excludedCount, repos, err := listReposForProject(params, *project.Name, gitClient)

		if err != nil {
			if len(params.SingleRepos) == 0 {
				fmt.Println("\r❌ Get Repos for each Project:", err)
				spin1.Stop()
				continue
			} else {
				errmessage := fmt.Sprintf(" Get Repo %s for Project %s %v", params.SingleRepos, *project.Name, err)
				spin1.Stop()
				return importantBranches, emptyRepos, NBRrepos, TotalBranches, totalexclude, cptarchiv, fmt.Errorf(errmessage)
			}
		}
		emptyRepos = emptyRepos + emptyOrArchivedCount
		totalexclude = totalexclude + excludedCount

		spin1.Stop()
		if emptyOrArchivedCount > 0 {
			NBRrepo = len(repos) + emptyOrArchivedCount
			fmt.Printf("\t  ✅ The number of %s found is: %d - Find empty %d:\n", message4, NBRrepo, emptyOrArchivedCount)
		} else {
			NBRrepo = len(repos)
			fmt.Printf("\t  ✅ The number of %s found is: %d\n", message4, NBRrepo)
		}

		for _, repo := range repos {

			largestRepoBranch, repobranches, brsize, err := analyzeRepoBranches(params, *project.Name, *repo.Name, gitClient, cpt, spin1)
			if err != nil {
				largestRepoBranch = *repo.DefaultBranch

			}

			importantBranches = append(importantBranches, ProjectBranch{
				Org:         params.Organization,
				ProjectKey:  *project.Name,
				RepoSlug:    *repo.Name,
				MainBranch:  largestRepoBranch,
				LargestSize: brsize,
			})
			TotalBranches += repobranches

			cpt++
		}

		NBRrepos += NBRrepo
		/*	} else {
			continue
		}*/
	}

	return importantBranches, emptyRepos, NBRrepos, TotalBranches, totalexclude, cptarchiv, nil

}

func listReposForProject(parms ParamsProjectAzure, projectKey string, gitClient git.Client) (int, int, int, []git.GitRepository, error) {
	var allRepos []git.GitRepository
	var archivedCount, emptyCount, excludedCount int

	// Get repositories
	repos, err := gitClient.GetRepositories(parms.Context, git.GetRepositoriesArgs{
		Project: &projectKey,
	})
	if err != nil {
		fmt.Println("Error get GetRepositories ")
		return 0, 0, 0, nil, err
	}

	for _, repo := range *repos {
		repoName := *repo.Name

		// check if exclude
		if isRepoExcluded(parms.Exclusionlist, projectKey, repoName) {
			excludedCount++
			continue
		}
		repoID := repo.Id.String()

		isEmpty, err := isRepoEmpty(parms.Context, gitClient, projectKey, repoID)
		if err != nil {
			return 0, 0, 0, nil, err
		}
		if isEmpty {
			emptyCount++
			continue
		}

		allRepos = append(allRepos, repo)
	}

	return archivedCount, emptyCount, excludedCount, allRepos, nil
}

func analyzeRepoBranches(parms ParamsProjectAzure, projectKey string, repo string, gitClient git.Client, cpt int, spin1 *spinner.Spinner) (string, int, int64, error) {

	var largestRepoBranch string
	var nbrbranch int
	var err error
	var brsize int64

	largestRepoBranch, brsize, nbrbranch, err = getMostImportantBranch(parms.Context, gitClient, projectKey, repo, parms.Period, parms.DefaultB)
	if err != nil {
		spin1.Stop()
		return "", 0, 1, err
	}

	spin1.Stop()

	// Print analysis summary
	fmt.Printf("\t\t✅ Repo %d: %s - Number of branches: %d - Largest Branch: %s\n", cpt, repo, nbrbranch, largestRepoBranch)

	return largestRepoBranch, nbrbranch, brsize, nil

}

func getMostImportantBranch(ctx context.Context, gitClient git.Client, projectID string, repoID string, periode int, DefaultB bool) (string, int64, int, error) {
	const REF = "refs/heads/"

	var mostImportantBranch string
	var maxCommits, nbrbranches, commitCount int
	var totalCommitSize int64
	var defaultBranch string
	var err1 error

	since := time.Now().AddDate(0, periode, 0)
	sinceStr := since.Format(time.RFC3339)

	// Get default branch
	repo, err1 := gitClient.GetRepository(ctx, git.GetRepositoryArgs{
		RepositoryId: &repoID,
		Project:      &projectID,
	})
	if err1 != nil {
		return "", 0, 0, err1
	}
	defaultBranch = *repo.DefaultBranch

	if !DefaultB {

		branches, err := gitClient.GetBranches(ctx, git.GetBranchesArgs{
			RepositoryId: &repoID,
			Project:      &projectID,
		})
		if err != nil {
			return "", 0, 0, err
		}

		for _, branch := range *branches {

			branchCommitSize := int64(0)

			commitCount, err = getCommitCount(ctx, gitClient, projectID, repoID, *branch.Name, sinceStr)
			if err != nil {
				return "", 0, 0, err
			}

			if commitCount > maxCommits {
				maxCommits = commitCount
				mostImportantBranch = strings.TrimPrefix(*branch.Name, REF)
				totalCommitSize = branchCommitSize
			}

			if maxCommits == 0 {
				mostImportantBranch = strings.TrimPrefix(defaultBranch, REF)
			}

			nbrbranches = len(*branches)

		}
	} else {
		commitCount, err1 = getCommitCount(ctx, gitClient, projectID, repoID, strings.TrimPrefix(defaultBranch, REF), sinceStr)
		if err1 != nil {
			return "", 0, 0, err1
		}
		if commitCount == 0 {
			totalCommitSize = int64(*repo.Size)
		} else {
			totalCommitSize = int64(commitCount)
		}

		mostImportantBranch = strings.TrimPrefix(defaultBranch, REF)
		nbrbranches = 1

	}

	return mostImportantBranch, totalCommitSize, nbrbranches, nil
}

func getCommitCount(ctx context.Context, gitClient git.Client, projectID string, repoID string, branchName string, sinceStr string) (int, error) {
	totalCommits := 0
	pages := 100
	skip := 0

	for {
		searchCriteria := git.GitQueryCommitsCriteria{
			ItemVersion: &git.GitVersionDescriptor{
				Version:        &branchName,
				VersionType:    &git.GitVersionTypeValues.Branch,
				VersionOptions: &git.GitVersionOptionsValues.None,
			},
			FromDate: &sinceStr,
			Top:      &pages,
			Skip:     &skip,
		}

		commits, err := gitClient.GetCommits(ctx, git.GetCommitsArgs{
			RepositoryId:   &repoID,
			Project:        &projectID,
			SearchCriteria: &searchCriteria,
		})
		if err != nil {
			return 0, err
		}

		totalCommits += len(*commits)

		if len(*commits) < pages {
			break
		}

		skip += pages
	}

	return totalCommits, nil
}
func SaveResult(result AnalysisResult) error {
	// Open or create the file
	file, err := os.Create("Results/config/analysis_result_azure.json")
	if err != nil {
		fmt.Println("❌ Error creating Analysis file:", err)
		return err
	}
	defer file.Close()

	// Create a JSON encoder
	encoder := json.NewEncoder(file)

	// Encode the result and write it to the file
	if err := encoder.Encode(result); err != nil {
		fmt.Println("❌ Error encoding JSON file <Results/config/analysis_result_azure.json> :", err)
		return err
	}

	fmt.Println("✅ Result saved successfully!")
	return nil
}
