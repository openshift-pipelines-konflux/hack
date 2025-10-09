package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	k "github.com/openshift-pipelines/hack/internal/konflux"
	"gopkg.in/yaml.v2"
)

func main() {
	configFile := "config/konflux.yaml"
	configDir := filepath.Dir(configFile)

	// Read the main konflux config using the generic readResource function
	config, err := readConfig(configDir)
	if err != nil {
		log.Fatal(err)
	}

	for _, version := range config.Versions {
		for _, applicationName := range config.Applications {
			// Read application using the generic readResource function
			application, err := readApplication(configDir, applicationName, &version)
			if err != nil {
				log.Fatal(err)
			}
			log.Printf("Loaded application: %s", application.Name)
			if err := k.GenerateConfig(application); err != nil {
				log.Fatal(err)
			}

		}
	}

	log.Printf("Done:")
}

// readResource reads any type of resource from YAML files
func readResource[T any](dir, resourceType, resourceName string) (T, error) {
	var result T

	filePath := filepath.Join(dir, resourceType, resourceName+".yaml")
	in, err := os.ReadFile(filePath)

	if err != nil {
		return result, err
	}

	if err := yaml.UnmarshalStrict(in, &result); err != nil {
		return result, fmt.Errorf("error while parsing config %s: %w", filePath, err)
	}

	return result, nil
}

// Helper functions using the generic readResource function
func readApplication(dir, applicationName string, version *k.Version) (k.Application, error) {

	log.Printf("Reading application: %s", applicationName)
	applicationConfig, err := readResource[k.ApplicationConfig](dir, "applications", applicationName)

	if err != nil {
		return k.Application{}, err
	}
	application := k.Application{
		Name:       applicationName,
		Components: []k.Component{},
		Version:    version,
	}

	for _, repoName := range applicationConfig.Repositories {
		repo, err := readRepository(dir, repoName, &application)
		if err != nil {
			return k.Application{}, err
		}
		application.Components = append(application.Components, repo.Components...)
		application.Repositories = append(application.Repositories, repo)

		log.Printf("Loaded repository: %s", repo.Name)
	}

	return application, nil
}

func updateRepository(repo *k.Repository, a k.Application) error {
	repo.Application = a

	repository := fmt.Sprintf("https://github.com/%s/%s.git", GithubOrg, repo.Name)
	repo.Url = repository

	branch := k.Branch{}

	if a.Version.Version == "next" {
		branch.Name = "main"
		branch.UpstreamBranch = "main"
	} else {
		branch.Name = "release-v" + a.Version.Version + ".x"
		branch.UpstreamBranch = "main"
	}
	repo.Branch = branch

	// Tekton
	if repo.Tekton == (k.Tekton{}) {
		repo.Tekton = k.Tekton{}
		if repo.Tekton.WatchedSources == "" {
			repo.Tekton.WatchedSources = `"upstream/***".pathChanged() || ".konflux/patches/***".pathChanged() || ".konflux/rpms/***".pathChanged()`
		}

	}

	return nil
}

// readRepository reads a repository resource from the repos directory
func readRepository(dir, repoName string, app *k.Application) (k.Repository, error) {
	repository, err := readResource[k.Repository](dir, "repos", repoName)
	if err != nil {
		return k.Repository{}, err
	}

	if err := updateRepository(&repository, *app); err != nil {
		return k.Repository{}, err
	}
	for i := range repository.Components {
		if err := UpdateComponent(&repository.Components[i], repository, *app); err != nil {
			return k.Repository{}, err
		}
	}
	return repository, err
}

// readConfig reads the main konflux config file
func readConfig(dir string) (k.Config, error) {
	return readResource[k.Config](dir, "", "konflux")
}
