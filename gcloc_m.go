package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/briandowns/spinner"
	"github.com/colussim/gcloc_m/internal/constants"

	//	getbibucket "github.com/colussim/gcloc_m/pkg/devops/getbitbucket"
	getbibucketdc "github.com/colussim/gcloc_m/pkg/devops/getbitbucketdc"
	"github.com/colussim/gcloc_m/pkg/devops/getgithub"
	"github.com/colussim/gcloc_m/pkg/devops/getgitlab"
	"github.com/colussim/gcloc_m/pkg/gcloc"
	"github.com/colussim/gcloc_m/pkg/utils"
)

type Repository struct {
	ID            int    `json:"id"`
	Name          string `json:"name"`
	DefaultBranch string `json:"default_branch"`
	Path          string `json:"path"`
}

type Project struct {
	KEY    string `json:"key"`
	ID     int    `json:"id"`
	Name   string `json:"name"`
	Public bool   `json:"public"`
	Type   string `json:"type"`
	Links  Links  `json:"links"`
}

type Links struct {
	Self []SelfLink `json:"self"`
}

type SelfLink struct {
	Href string `json:"href"`
}

type Config struct {
	Platforms map[string]interface{} `json:"platforms"`
}

type Report struct {
	TotalFiles      int `json:",omitempty"`
	TotalLines      int
	TotalBlankLines int
	TotalComments   int
	TotalCodeLines  int
	Results         interface{}
}

type Result struct {
	TotalFiles      int           `json:"TotalFiles"`
	TotalLines      int           `json:"TotalLines"`
	TotalBlankLines int           `json:"TotalBlankLines"`
	TotalComments   int           `json:"TotalComments"`
	TotalCodeLines  int           `json:"TotalCodeLines"`
	Results         []LanguageRes `json:"Results"`
}

type LanguageRes struct {
	Language   string `json:"Language"`
	Files      int    `json:"Files"`
	Lines      int    `json:"Lines"`
	BlankLines int    `json:"BlankLines"`
	Comments   int    `json:"Comments"`
	CodeLines  int    `json:"CodeLines"`
}

type Repository1 interface {
	GetID() int
	GetName() string
	GetPath() string
}

type Project1 interface {
	GetID() int
	GetName() string
}

func (p Project) GetName() string {
	return p.Name
}

func (p Project) GetId() int {
	return p.ID
}

type GithubRepository struct {
	ID            int    `json:"id"`
	Name          string `json:"name"`
	DefaultBranch string `json:"default_branch"`
	Path          string `json:"full_name"`
	SizeR         int64  `json:"size"`
}

func (r GithubRepository) GetName() string {
	return r.Name
}
func (r GithubRepository) GetPath() string {
	return r.Path
}
func (r GithubRepository) GetID() int {
	return r.ID
}

// Implémentation pour getgitlab.Repository
type GitlabRepository struct {
	ID            int    `json:"id"`
	Name          string `json:"name"`
	DefaultBranch string `json:"default_branch"`
	Path          string `json:"path_with_namespace"`
	Empty         bool   `json:"empty_repo"`
}

func (r GitlabRepository) GetName() string {
	return r.Name
}
func (r GitlabRepository) GetPath() string {
	return r.Path
}
func (r GitlabRepository) GetID() int {
	return r.ID
}

func formatCodeLines(numLines float64) string {
	if numLines >= 1000000 {
		return fmt.Sprintf("%.2fM", numLines/1000000)
	} else if numLines >= 1000 {
		return fmt.Sprintf("%.2fK", numLines/1000)
	} else {
		return fmt.Sprintf("%.0f", numLines)
	}
}

func getFileNameIfExists(filePath string) string {
	_, err := os.Stat(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			//The file does not exist
			return "0"
		} else {
			// Check file
			fmt.Printf("❌ Error check file exclusion: %v\n", err)
			return "0"
		}
	} else {
		return filePath
	}
}

func LoadConfig(filename string) (Config, error) {
	var config Config

	// Lire le contenu du fichier de configuration
	data, err := os.ReadFile(filename)
	if err != nil {
		return config, fmt.Errorf("❌ failed to read config file: %w", err)
	}

	if err := json.Unmarshal(data, &config); err != nil {
		return config, fmt.Errorf("❌ failed to parse config JSON: %w", err)
	}

	return config, nil
}

// Parse Result Files in JSON Format
func parseJSONFile(filePath, reponame string) int {
	file, err := os.ReadFile(filePath)
	if err != nil {
		fmt.Println("❌ Error reading file:", err)
	}

	var report Report
	err = json.Unmarshal(file, &report)
	if err != nil {
		fmt.Println("❌ Error parsing JSON:", err)
	}

	//fmt.Printf("\nTotal Lines Of Code : %d\n\n", report.TotalCodeLines)

	return report.TotalCodeLines
}

// Analyse Repositories bitbucket DC
func AnalyseReposListB(DestinationResult string, user string, AccessToken string, Protocol string, URL string, DevOps string, repolist []getbibucketdc.ProjectBranch) (cpt int) {

	//var pathBranches string
	trimmedURL := strings.TrimPrefix(URL, "http://")

	fmt.Print("\n🔎 Analysis of Repos ...\n")

	spin := spinner.New(spinner.CharSets[35], 100*time.Millisecond)
	spin.Color("green", "bold")
	messageF := ""
	spin.FinalMSG = messageF

	for _, project := range repolist {

		pathToScan := fmt.Sprintf("%s://%s:%s@%sscm/%s/%s.git", Protocol, user, AccessToken, trimmedURL, project.ProjectKey, project.RepoSlug)
		outputFileName := fmt.Sprintf("Result_%s_%s_%s", project.ProjectKey, project.RepoSlug, project.MainBranch)

		params := gcloc.Params{
			Path:              pathToScan,
			ByFile:            false,
			ExcludePaths:      []string{},
			ExcludeExtensions: []string{},
			IncludeExtensions: []string{},
			OrderByLang:       false,
			OrderByFile:       false,
			OrderByCode:       false,
			OrderByLine:       false,
			OrderByBlank:      false,
			OrderByComment:    false,
			Order:             "DESC",
			OutputName:        outputFileName,
			OutputPath:        DestinationResult,
			ReportFormats:     []string{"json"},
			Branch:            project.MainBranch,
		}
		MessB := fmt.Sprintf("   Extracting files from repo : %s ", project.RepoSlug)
		spin.Suffix = MessB
		spin.Start()

		gc, err := gcloc.NewGCloc(params, constants.Languages)
		if err != nil {
			fmt.Println("\nError Analyse Repositories: ", err)
			os.Exit(1)
		}
		//fmt.Println("\r ")
		gc.Run()
		cpt++

		// Remove Repository Directory
		err1 := os.RemoveAll(gc.Repopath)
		if err != nil {
			fmt.Printf("❌ Error deleting Repository Directory: %v\n", err1)
			return
		}

		spin.Stop()
		fmt.Printf("\t✅ The repository <%s> has been analyzed\n", project.RepoSlug)
	}
	return cpt
}

func AnalyseReposList(DestinationResult string, Users string, AccessToken string, DevOps string, Organization string, repolist []Repository1) (cpt int) {
	var pathBranches string
	for _, repo := range repolist {
		fmt.Printf("\nAnalyse Repository : %s\n", repo.GetName())

		// Get All Branches in Repository
		branches, err := getgitlab.GetRepositoryBranches(AccessToken, repo.GetName(), repo.GetID())
		if err != nil {
			fmt.Println("\nError Analyse Repository Branches: ", err)
			os.Exit(1)
		}

		for _, branch := range branches {
			fmt.Printf("    - %s\n", branch.Name)

			pathBranches = fmt.Sprintf("?ref=%s", branch.Name)

			pathToScan := fmt.Sprintf("https://%s:%s@%s.com/%s%s", Users, AccessToken, DevOps, repo.GetPath(), pathBranches)
			fmt.Println("Scan PATH :", pathToScan)
			outputFileName := fmt.Sprintf("Result_%s", repo.GetName())

			params := gcloc.Params{
				Path:              pathToScan,
				ByFile:            false,
				ExcludePaths:      []string{},
				ExcludeExtensions: []string{},
				IncludeExtensions: []string{},
				OrderByLang:       false,
				OrderByFile:       false,
				OrderByCode:       false,
				OrderByLine:       false,
				OrderByBlank:      false,
				OrderByComment:    false,
				Order:             "DESC",
				OutputName:        outputFileName,
				OutputPath:        DestinationResult,
				ReportFormats:     []string{"json"},
			}

			gc, err := gcloc.NewGCloc(params, constants.Languages)
			if err != nil {
				fmt.Println("\nError Analyse Repositories: ", err)
				os.Exit(1)
			}

			gc.Run()
			cpt++

			// Remove Repository Directory
			err1 := os.RemoveAll(gc.Repopath)
			if err != nil {
				fmt.Printf("Error deleting Repository Directory: %v\n", err1)
				return
			}

		}
	}
	return cpt
}

func AnalyseRun(params gcloc.Params, reponame string) {
	gc, err := gcloc.NewGCloc(params, constants.Languages)
	if err != nil {
		fmt.Println("\n❌ Error Analyse Repositories: ", err)
		os.Exit(1)
	}

	gc.Run()
}

func AnalyseRepo(DestinationResult string, Users string, AccessToken string, DevOps string, Organization string, reponame string) (cpt int) {

	//pathToScan := fmt.Sprintf("git::https://%s@%s.com/%s/%s", AccessToken, DevOps, Organization, reponame)
	pathToScan := fmt.Sprintf("https://%s:%s@%s.com/%s/%s", Users, AccessToken, DevOps, Organization, reponame)

	outputFileName := fmt.Sprintf("Result_%s", reponame)
	params := gcloc.Params{
		Path:              pathToScan,
		ByFile:            false,
		ExcludePaths:      []string{},
		ExcludeExtensions: []string{},
		IncludeExtensions: []string{},
		OrderByLang:       false,
		OrderByFile:       false,
		OrderByCode:       false,
		OrderByLine:       false,
		OrderByBlank:      false,
		OrderByComment:    false,
		Order:             "DESC",
		OutputName:        outputFileName,
		OutputPath:        DestinationResult,
		ReportFormats:     []string{"json"},
	}
	gc, err := gcloc.NewGCloc(params, constants.Languages)
	if err != nil {
		fmt.Println("\nError Analyse Repositories: ", err)
		os.Exit(1)
	}

	gc.Run()
	cpt++

	// Remove Repository Directory
	err1 := os.RemoveAll(gc.Repopath)
	if err != nil {
		fmt.Printf("❌ Error deleting Repository Directory: %v\n", err1)
		return
	}

	return cpt
}

func main() {

	var maxTotalCodeLines int
	var maxProject, maxRepo string
	var NumberRepos int
	var startTime time.Time

	devopsFlag := flag.String("devops", "", "Specify the DevOps platform")
	flag.Parse()

	if *devopsFlag == "" {
		fmt.Println("\n❌ Please specify the DevOps platform using the --devops flag : <BitBucketSRV>||<BitBucket>||<Github>||<Gitlab>||<Azure>||<File>")
		fmt.Println("✅ Example for BitBucket server : gcloc_m.go --devops BitBucketSRV")
		os.Exit(1)
	}
	AppConfig, err := LoadConfig("config.json")
	if err != nil {
		log.Fatalf("\n❌ Failed to load config: %s", err)
		os.Exit(1)
	}

	platformConfig, ok := AppConfig.Platforms[*devopsFlag].(map[string]interface{})
	if !ok {
		fmt.Printf("\n❌ Configuration for DevOps platform '%s' not found\n", *devopsFlag)
		fmt.Println("✅ the --devops flag is : <BitBucketSRV>||<BitBucket>||<Github>||<Gitlab>||<Azure>||<File>")
		os.Exit(1)
	}

	fmt.Printf("\n✅ Using configuration for DevOps platform '%s'\n", *devopsFlag)

	pwd, err := os.Getwd()
	if err != nil {
		fmt.Println("Error:", err)
	}
	DestinationResult := pwd + "/Results"
	_, err = os.Stat(DestinationResult)
	if err == nil {

		fmt.Printf("❗️ Directory <'%s'> already exists. Do you want to delete it? (y/n): ", DestinationResult)
		var response string
		fmt.Scanln(&response)

		if response == "y" || response == "Y" {
			err := os.RemoveAll(DestinationResult)
			if err != nil {
				fmt.Printf("❌ Error deleting directory: %s\n", err)
				os.Exit(1)
			}
			if err := os.MkdirAll(DestinationResult, os.ModePerm); err != nil {
				panic(err)
			}
		} else {
			os.Exit(1)
		}

	} else if os.IsNotExist(err) {
		if err := os.MkdirAll(DestinationResult, os.ModePerm); err != nil {
			panic(err)
		}

	}
	fmt.Printf("\n")

	GlobalReport := DestinationResult + "/GlobalReport.txt"
	// Create Global Report File
	file, err := os.Create(GlobalReport)
	if err != nil {
		fmt.Println("❌ Error creating file:", err)
		return
	}
	defer file.Close()

	switch devops := platformConfig["DevOps"].(string); devops {
	case "github":
		var EmptyR = 0
		var fileexclusion = ".clocignore"
		repositories, err := getgithub.GetRepoGithubList(platformConfig["AccessToken"].(string), platformConfig["Organization"].(string))
		if err != nil {
			fmt.Printf("Error Get Info Repositories in organization '%s' : '%s'", platformConfig["Organization"].(string), err)
			return
		}
		var repoList []Repository1

		for _, repo := range repositories {

			repoItem := GithubRepository{
				ID:            repo.ID,
				Name:          repo.Name,
				DefaultBranch: repo.DefaultBranch,
				Path:          repo.Path,
				SizeR:         repo.SizeR,
			}
			if repo.SizeR > 0 {
				ExcludeRepo, _ := utils.CheckCLOCignoreFile(fileexclusion, repo.Name)
				if !ExcludeRepo {
					repoList = append(repoList, repoItem)
				} else {
					fmt.Printf("\nRepository '%s' in Organization: '%s' is not Analyse because it is excluded\n", repo.Name, platformConfig["Organization"].(string))
					EmptyR = EmptyR + 1
				}
			} else {
				fmt.Print("\nRepository '%s' in Organization: '%s' is not Analyse because it is empty\n", repo.Name, platformConfig["Organization"].(string))
				EmptyR = EmptyR + 1
			}
		}

		NumberRepos1 := int(uintptr(len(repositories))) - EmptyR
		fmt.Printf("\nAnalyse '%s' Repositories('%d') in Organization: '%s'\n", platformConfig["DevOps"].(string), NumberRepos1, platformConfig["Organization"].(string))

		NumberRepos = AnalyseReposList(DestinationResult, platformConfig["Users"].(string), platformConfig["AccessToken"].(string), platformConfig["DevOps"].(string), platformConfig["Organization"].(string), repoList)

	case "gitlab":
		var EmptyR = 0
		var fileexclusion = ".clocignore"
		repositories, err := getgitlab.GetRepoGitlabList(platformConfig["AccessToken"].(string), platformConfig["Organization"].(string))
		if err != nil {
			fmt.Printf("❌ Error Get Info Repositories in organization '%s' : '%s'", platformConfig["Organization"].(string), err)
			return
		}

		var repoList []Repository1
		for _, repo := range repositories {
			repoItem := GitlabRepository{
				ID:            repo.ID,
				Name:          repo.Name,
				DefaultBranch: repo.DefaultBranch,
				Path:          repo.Path,
				Empty:         repo.Empty,
			}
			if !repo.Empty {
				ExcludeRepo, _ := utils.CheckCLOCignoreFile(fileexclusion, repo.Name)
				if !ExcludeRepo {
					repoList = append(repoList, repoItem)
				} else {
					fmt.Print("\nRepository '%s' in Organization: '%s' is not Analyse because it is excluded\n", repo.Name, platformConfig["Organization"].(string))
					EmptyR = EmptyR + 1
				}
			} else {
				fmt.Printf("\nRepository '%s' in Organization:'%s' is not Analyse because it is empty\n", repo.Name, platformConfig["Organization"].(string))
				EmptyR = EmptyR + 1
			}
		}

		NumberRepos1 := int(uintptr(len(repositories))) - EmptyR
		fmt.Printf("\nAnalyse '%s' Repositories('%d') in Organization: '%s'\n", platformConfig["DevOps"].(string), NumberRepos1, platformConfig["Organization"].(string))

		NumberRepos = AnalyseReposList(DestinationResult, platformConfig["Users"].(string), platformConfig["AccessToken"].(string), platformConfig["DevOps"].(string), platformConfig["Organization"].(string), repoList)

	case "bitbucket_dc":

		var fileexclusion = platformConfig["FileExclusion"].(string)
		fileexclusionEX := getFileNameIfExists(fileexclusion)

		startTime = time.Now()
		projects, err := getbibucketdc.GetProjectBitbucketList(platformConfig["Url"].(string), platformConfig["Baseapi"].(string), platformConfig["Apiver"].(string), platformConfig["AccessToken"].(string), fileexclusionEX, platformConfig["Project"].(string), platformConfig["Repos"].(string))
		if err != nil {
			fmt.Printf("❌ Error Get Info Projects in Bitbucket server '%s' : ", err)
			os.Exit(1)
		}

		// Run scanning repositories
		NumberRepos = AnalyseReposListB(DestinationResult, platformConfig["Users"].(string), platformConfig["AccessToken"].(string), platformConfig["Protocol"].(string), platformConfig["Url"].(string), platformConfig["DevOps"].(string), projects)

	case "bitbucket":
		//var fileexclusion = platformConfig["FileExclusion"].(string)
		/*	fileexclusionEX := getFileNameIfExists(fileexclusion)

			startTime = time.Now()

			projects1, err := getbibucket.GetProjectBitbucketListCloud(platformConfig["Url"].(string), platformConfig["Baseapi"].(string), platformConfig["Apiver"].(string), platformConfig["AccessToken"].(string), platformConfig["Workspace"].(string), fileexclusionEX, platformConfig["Project"].(string), platformConfig["Repos"].(string))
			if err != nil {
				fmt.Printf("❌ Error Get Info Projects in Bitbucket server '%s' : ", err)
				return
			}
			fmt.Println("Taille de pr", len(projects1))
			for _, allproject := range projects1 {

				fmt.Println("\n✅ Projet KEY: , Projet Name: \n", allproject.Key, allproject.Name)
				fmt.Println()
			} */

	}
	fmt.Print("\n🔎 Analyse Report ...\n")
	spin := spinner.New(spinner.CharSets[35], 100*time.Millisecond)
	spin.Suffix = " Analyse Report..."
	spin.Color("green", "bold")
	spin.Start()

	// List files in the directory
	files, err := os.ReadDir(DestinationResult)
	if err != nil {
		fmt.Println("❌ Error listing files:", err)
		os.Exit(1)
	}

	// Initialize the sum of TotalCodeLines
	totalCodeLinesSum := 0

	for _, file := range files {
		// Check if the file is a JSON file
		if !file.IsDir() && strings.HasSuffix(file.Name(), ".json") {
			// Read contents of JSON file
			filePath := filepath.Join(DestinationResult, file.Name())
			jsonData, err := os.ReadFile(filePath)
			if err != nil {
				fmt.Printf("\n❌ Error reading file %s: %v\n", file.Name(), err)
				continue
			}

			// Parse JSON content into a Result structure
			var result Result
			err = json.Unmarshal(jsonData, &result)
			if err != nil {
				fmt.Printf("\n❌ Error parsing JSON contents of file %s: %v\n", file.Name(), err)
				continue
			}

			totalCodeLinesSum += result.TotalCodeLines

			// Check if this repo has a higher TotalCodeLines than the current maximum
			if result.TotalCodeLines > maxTotalCodeLines {
				maxTotalCodeLines = result.TotalCodeLines
				// Extract project and repo name from file name
				parts := strings.Split(strings.TrimSuffix(file.Name(), ".json"), "_")
				maxProject = parts[1]
				maxRepo = parts[2]
			}
		}
	}

	maxTotalCodeLines1 := formatCodeLines(float64(maxTotalCodeLines))
	totalCodeLinesSum1 := formatCodeLines(float64(totalCodeLinesSum))
	spin.Stop()

	endTime := time.Now()
	duration := endTime.Sub(startTime)

	hours := int(duration.Hours())
	minutes := int(duration.Minutes()) % 60
	seconds := int(duration.Seconds()) % 60

	message0 := fmt.Sprintf("\n✅ Number of Repository analyzed in Organization <%s> is %d \n", platformConfig["Organization"].(string), NumberRepos)
	message1 := fmt.Sprintf("✅ The repository with the largest line of code is in project <%s> the repo name is <%s> with <%s> lines of code\n", maxProject, maxRepo, maxTotalCodeLines1)
	message2 := fmt.Sprintf("✅ The total sum of lines of code in Organization <%s> is : %s Lines of Code\n", platformConfig["Organization"].(string), totalCodeLinesSum1)
	message3 := message0 + message1 + message2

	fmt.Println(message3)
	fmt.Println("\n✅ Reports are located in the <'Results'> directory")
	fmt.Printf("\n✅ Time elapsed : %02d:%02d:%02d\n", hours, minutes, seconds)

	// Write message in Gobal Report File
	_, err = file.WriteString(message3)
	if err != nil {
		fmt.Println("Error writing to file:", err)
		return
	}
}
