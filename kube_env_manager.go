package minienv

import (
	"strings"
	"log"
	"net/http"
	"io/ioutil"
	"gopkg.in/yaml.v2"
	"strconv"
	"fmt"
)

type KubeEnvManager interface {
	GetProvisionerJobYamlTemplate() (string)
	GetProvisionVolumeSize() string
	GetProvisionImages() (string)
	GetPersistentVolumeStorageClass() (string)
	UseHostPathPersistentVolumes() (bool)
	GetPersistentVolumeYamlTemplate() (string)
	GetPersistentVolumeClaimYamlTemplate() (string)
	GetServiceYamlTemplate() (string)
	GetDeploymentYamlTemplate() (string)
	GetDeploymentDetails(envId string, claimToken string, envConfigPath string, repo *DeploymentRepo) (*DeploymentDetails, error)
	GetDeploymentYaml(envId string, claimToken string, minienvVersion string, minienvImage string, nodeNameOverride string, nodeHostProtocol string, storageDriver string, repo *DeploymentRepo, details *DeploymentDetails, envVars map[string]string) (string)
	GetServiceYaml(envId string, claimToken string, details *DeploymentDetails) (string)
	GetPersistentVolumeYaml(envId string) (string)
	GetPersistentVolumeClaimYaml(envId string) (string)
}

type BaseKubeEnvManager struct {
	ProvisionerJobYamlTemplate string
	ProvisionVolumeSize string
	ProvisionImages string
	PersistentVolumeStorageClass string
	PersistentVolumeHostPath bool
	PersistentVolumeYamlTemplate string
	PersistentVolumeClaimYamlTemplate string
	ServiceYamlTemplate string
	DeploymentYamlTemplate string
}

func (envManager *BaseKubeEnvManager) GetProvisionerJobYamlTemplate() (string) {
	return envManager.ProvisionerJobYamlTemplate
}

func (envManager *BaseKubeEnvManager) GetProvisionVolumeSize() string {
	return envManager.ProvisionVolumeSize
}

func (envManager *BaseKubeEnvManager) GetProvisionImages() (string) {
	return envManager.ProvisionImages
}

func (envManager *BaseKubeEnvManager) GetPersistentVolumeStorageClass() (string) {
	return envManager.PersistentVolumeStorageClass
}

func (envManager *BaseKubeEnvManager) UseHostPathPersistentVolumes() (bool) {
	return envManager.PersistentVolumeHostPath
}

func (envManager *BaseKubeEnvManager) GetPersistentVolumeYamlTemplate() (string) {
	return envManager.PersistentVolumeYamlTemplate
}

func (envManager *BaseKubeEnvManager) GetPersistentVolumeClaimYamlTemplate() (string) {
	return envManager.PersistentVolumeClaimYamlTemplate
}

func (envManager *BaseKubeEnvManager) GetServiceYamlTemplate() (string) {
	return envManager.ServiceYamlTemplate
}

func (envManager *BaseKubeEnvManager) GetDeploymentYamlTemplate() (string) {
	return envManager.DeploymentYamlTemplate
}

func (envManager *BaseKubeEnvManager) GetDeploymentDetails(envId string, claimToken string, envConfigPath string, repo *DeploymentRepo) (*DeploymentDetails, error) {
	envConfig, err := downloadEnvConfig(envConfigPath, repo.Repo, repo.Branch, repo.Username, repo.Password)
	if err != nil {
		log.Println("Error downloading minienv.json", err)
	}
	var tabs []*DeploymentTab
	if envConfig == nil || envConfig.Runtime == nil || envConfig.Runtime.Platform == "" {
		// download docker-compose file if platform not specified in minienv config
		// first try yml, then yaml
		dockerComposeUrl := getDownloadUrl("docker-compose.yml", repo.Repo, repo.Branch, repo.Username, repo.Password)
		log.Printf("Downloading docker-compose file from '%s'...\n", dockerComposeUrl)
		client := getHttpClient()
		req, err := http.NewRequest("GET", dockerComposeUrl, nil)
		resp, err := client.Do(req)
		if err != nil || resp.StatusCode != 200 {
			log.Println("Error downloading docker-compose.yml: ", err)
			dockerComposeUrl := getDownloadUrl("docker-compose.yaml", repo.Repo, repo.Branch, repo.Username, repo.Password)
			req, err = http.NewRequest("GET", dockerComposeUrl, nil)
			resp, err = client.Do(req)
			if err != nil || resp.StatusCode != 200 {
				log.Println("Error downloading docker-compose.yaml: ", err)
				return nil, err
			}
		}
		m := make(map[interface{}]interface{})
		data, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			log.Println("Error downloading docker-compose file: ", err)
			return nil, err
		} else {
			err = yaml.Unmarshal(data, &m)
			if err != nil {
				log.Println("Error parsing docker-compose file: ", err)
				return nil, err
			} else {
				for k, v := range m {
					populateTabs(v, &tabs, k.(string))
				}
			}
		}
	}
	// populate docker compose names and paths
	if envConfig != nil && envConfig.Metadata != nil && envConfig.Metadata.AppTabs != nil && len(*envConfig.Metadata.AppTabs) > 0 {
		for _, appTab := range *envConfig.Metadata.AppTabs {
			tabUpdated := false
			// update the original docker compose port if it exists
			for _, tab := range tabs {
				if tab.Port == appTab.Port && tab.AppTab == nil {
					tab.AppTab = &appTab
					tab.Hide = appTab.Hide
					if appTab.Name != "" {
						tab.Name = appTab.Name
					}
					if appTab.Path != "" {
						tab.Path = appTab.Path
					}
					tabUpdated = true
					break
				}
			}
			if ! tabUpdated {
				// add other docker compose ports
				tab := &DeploymentTab{}
				tab.AppTab = &appTab
				tab.Hide = appTab.Hide
				tab.Port = appTab.Port
				tab.Name = strconv.Itoa(appTab.Port)
				tabs = append(tabs, tab)
				if appTab.Name != "" {
					tab.Name = appTab.Name
				}
				if appTab.Path != "" {
					tab.Path = appTab.Path
				}
			}
		}
	}
	// ports
	details := &DeploymentDetails{}
	details.EnvConfig = envConfig
	details.LogPort = DefaultLogPort
	details.EditorPort = DefaultEditorPort
	details.AppPort = DefaultProxyPort
	details.NodeHostName = NodeHostName
	details.EnvId = envId
	details.ClaimToken = claimToken
	details.LogUrl = fmt.Sprintf("%s://%s-%s.%s", NodeHostProtocol, "$sessionId", details.LogPort, details.NodeHostName)
	details.EditorUrl = fmt.Sprintf("%s://%s-%s.%s", NodeHostProtocol, "$sessionId", details.EditorPort, details.NodeHostName)
	if envConfig != nil && envConfig.Metadata != nil && envConfig.Metadata.EditorTab != nil {
		if envConfig.Metadata.EditorTab.Hide {
			details.EditorUrl = ""
		}
	}
	// add tab based on port if no tabs already provided
	if len(tabs) == 0 && envConfig != nil && envConfig.Runtime != nil && envConfig.Runtime.Port > 0 {
		tab := &DeploymentTab{}
		tab.Hide = false
		tab.Port = envConfig.Runtime.Port
		tab.Name = strconv.Itoa(envConfig.Runtime.Port)
		tab.Path = "/"
		tabs = append(tabs, tab)
	}
	for _, tab := range tabs {
		tab.Url = fmt.Sprintf("%s://%s-%s-%d.%s%s", NodeHostProtocol, "$sessionId", details.AppPort, tab.Port, details.NodeHostName, tab.Path)
	}
	details.Tabs = &tabs
	return details, nil
}

func (envManager *BaseKubeEnvManager) GetDeploymentYaml(envId string, claimToken string, minienvVersion string, minienvImage string, nodeNameOverride string, nodeHostProtocol string, storageDriver string, repo *DeploymentRepo, details *DeploymentDetails, envVars map[string]string) (string) {
	envVarsYaml := ""
	if envVars != nil {
		first := true
		for k, v := range envVars {
			if ! first {
				envVarsYaml += "\n"
			} else {
				first = false
			}
			envVarsYaml += "          - name: " + k
			envVarsYaml += "\n            value: \"" + v + "\""
		}
	}
	//platform := ""
	//platformCommand := ""
	//platformPort := ""
	editorSrcDir := ""
	if details.EnvConfig != nil {
		//if details.EnvConfig.Runtime != nil && details.EnvConfig.Runtime.Platform != "" {
		//	platform = details.EnvConfig.Runtime.Platform
		//	platformCommand = details.EnvConfig.Runtime.Command
		//	if details.EnvConfig.Runtime.Port > 0 {
		//		platformPort = strconv.Itoa(details.EnvConfig.Runtime.Port)
		//	} else if len(*details.Tabs) > 0 {
		//		tabs := *details.Tabs
		//		platformPort = strconv.Itoa(tabs[0].Port)
		//	}
		//}
		if details.EnvConfig.Metadata != nil && details.EnvConfig.Metadata.EditorTab != nil {
			editorSrcDir = details.EnvConfig.Metadata.EditorTab.SrcDir
		}
	}
	deployment := envManager.GetDeploymentYamlTemplate()
	deployment = strings.Replace(deployment, VarMinienvVersion, minienvVersion, -1)
	deployment = strings.Replace(deployment, VarMinienvImage, minienvImage, -1)
	deployment = strings.Replace(deployment, VarMinienvNodeNameOverride, nodeNameOverride, -1)
	deployment = strings.Replace(deployment, VarMinienvNodeHostProtocol, nodeHostProtocol, -1)
	deployment = strings.Replace(deployment, VarAllowOrigin, allowOrigin, -1)
	deployment = strings.Replace(deployment, VarStorageDriver, storageDriver, -1)
	//deployment = strings.Replace(deployment, VarMinienvPlatformPort, platformPort, -1) // this must be replaced before VarMinienvPlatform
	//deployment = strings.Replace(deployment, VarMinienvPlatformCommand, platformCommand, -1) // this must be replaced before VarMinienvPlatform
	//deployment = strings.Replace(deployment, VarMinienvPlatform, platform, -1)
	deployment = strings.Replace(deployment, VarGitRepoWithCreds, getUrlWithCredentials(repo.Repo, repo.Username, repo.Password), -1) // make sure this replace is done before gitRepo
	deployment = strings.Replace(deployment, VarGitRepo, repo.Repo, -1)
	deployment = strings.Replace(deployment, VarGitBranch, repo.Branch, -1)
	deployment = strings.Replace(deployment, VarProxyPort, details.AppPort, -1)
	deployment = strings.Replace(deployment, VarLogPort, details.LogPort, -1)
	deployment = strings.Replace(deployment, VarEditorPort, details.EditorPort, -1)
	deployment = strings.Replace(deployment, VarEditorSrcDir, editorSrcDir, -1)
	deployment = strings.Replace(deployment, VarDeploymentName, getEnvDeploymentName(envId), -1)
	deployment = strings.Replace(deployment, VarAppLabel, getEnvAppLabel(envId, claimToken), -1)
	deployment = strings.Replace(deployment, VarClaimToken, claimToken, -1)
	deployment = strings.Replace(deployment, VarEnvDetails, deploymentDetailsToString(details), -1)
	deployment = strings.Replace(deployment, VarEnvVars, envVarsYaml, -1)
	deployment = strings.Replace(deployment, VarPvcName, getPersistentVolumeClaimName(envId), -1)
	return deployment
}

func (envManager *BaseKubeEnvManager) GetServiceYaml(envId string, claimToken string, details *DeploymentDetails) (string) {
	service := envManager.GetServiceYamlTemplate()
	service = strings.Replace(service, VarServiceName, getEnvServiceName(envId, claimToken), -1)
	service = strings.Replace(service, VarAppLabel, getEnvAppLabel(envId, claimToken), -1)
	service = strings.Replace(service, VarLogPort, details.LogPort, -1)
	service = strings.Replace(service, VarEditorPort, details.EditorPort, -1)
	service = strings.Replace(service, VarProxyPort, details.AppPort, -1)
	return service
}

func (envManager *BaseKubeEnvManager) GetPersistentVolumeYaml(envId string) (string) {
	pv := envManager.GetPersistentVolumeYamlTemplate()
	pv = strings.Replace(pv, VarPvName, getPersistentVolumeName(envId), -1)
	pv = strings.Replace(pv, VarPvPath, getPersistentVolumePath(envId), -1)
	return pv
}

func (envManager *BaseKubeEnvManager) GetPersistentVolumeClaimYaml(envId string) (string) {
	pvc := envManager.GetPersistentVolumeClaimYamlTemplate()
	pvc = strings.Replace(pvc, VarPvSize, envManager.GetPersistentVolumeStorageClass(), -1)
	pvc = strings.Replace(pvc, VarPvcName, getPersistentVolumeClaimName(envId), -1)
	pvc = strings.Replace(pvc, VarPvcStorageClass, envManager.GetPersistentVolumeStorageClass(), -1)
	return pvc
}