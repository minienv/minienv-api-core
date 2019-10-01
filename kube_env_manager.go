package minienv

import (
	"strings"
	"log"
	"net/http"
	"io/ioutil"
	"gopkg.in/yaml.v2"
	"fmt"
	"encoding/json"
	"strconv"
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
	GetDeploymentTabsFromDockerCompose(session *Session, repo *DeploymentRepo) (*[]*DeploymentTab, error)
	GetDeploymentDetails(session *Session, envId string, claimToken string, repo *DeploymentRepo) (*DeploymentDetails, error)
	GetDeploymentYaml(session *Session, template string, details *DeploymentDetails, detailsString string, minienvVersion string, nodeNameOverride string, nodeHostProtocol string, storageDriver string, repo *DeploymentRepo, envVars map[string]string) (string)
	GetServiceYaml(session *Session, template string, details *DeploymentDetails) (string)
	GetPersistentVolumeYaml(template string, envId string, storageSize string) (string)
	GetPersistentVolumeClaimYaml(template string, envId string, storageSize string, storageClass string) (string)
	SerializeDeploymentDetails(details *DeploymentDetails) (string)
	DeserializeDeploymentDetails(detailsStr string) (*DeploymentDetails)
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

func (baseEnvManager *BaseKubeEnvManager) GetProvisionerJobYamlTemplate() (string) {
	return baseEnvManager.ProvisionerJobYamlTemplate
}

func (baseEnvManager *BaseKubeEnvManager) GetProvisionVolumeSize() string {
	return baseEnvManager.ProvisionVolumeSize
}

func (baseEnvManager *BaseKubeEnvManager) GetProvisionImages() (string) {
	return baseEnvManager.ProvisionImages
}

func (baseEnvManager *BaseKubeEnvManager) GetPersistentVolumeStorageClass() (string) {
	return baseEnvManager.PersistentVolumeStorageClass
}

func (baseEnvManager *BaseKubeEnvManager) UseHostPathPersistentVolumes() (bool) {
	return baseEnvManager.PersistentVolumeHostPath
}

func (baseEnvManager *BaseKubeEnvManager) GetPersistentVolumeYamlTemplate() (string) {
	return baseEnvManager.PersistentVolumeYamlTemplate
}

func (baseEnvManager *BaseKubeEnvManager) GetPersistentVolumeClaimYamlTemplate() (string) {
	return baseEnvManager.PersistentVolumeClaimYamlTemplate
}

func (baseEnvManager *BaseKubeEnvManager) GetServiceYamlTemplate() (string) {
	return baseEnvManager.ServiceYamlTemplate
}

func (baseEnvManager *BaseKubeEnvManager) GetDeploymentYamlTemplate() (string) {
	return baseEnvManager.DeploymentYamlTemplate
}

func (baseEnvManager *BaseKubeEnvManager) GetDeploymentTabsFromDockerCompose(_ *Session, repo *DeploymentRepo) (*[]*DeploymentTab, error) {
	var tabs []*DeploymentTab
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
	return &tabs, nil
}

func (baseEnvManager *BaseKubeEnvManager) GetAvailableDeploymentPort(port int, tabs *[]*DeploymentTab, otherPorts []int)  int {
	for true {
		used := false
		if otherPorts != nil {
			for _, otherPort := range otherPorts {
				if otherPort == port {
					port = port + 1
					used = true
					break
				}
			}
		}
		if ! used && tabs != nil {
			for _, tab := range *tabs {
				if tab.Port == port {
					port = port + 1
					used = true
					break
				}
			}
		}
		if used {
			continue
		} else {
			return port
		}
	}
	return port
}

func (baseEnvManager *BaseKubeEnvManager) GetDeploymentDetails(session *Session, envId string, claimToken string, repo *DeploymentRepo) (*DeploymentDetails, error) {
	tabs, err := baseEnvManager.GetDeploymentTabsFromDockerCompose(session, repo)
	if err != nil {
		return nil, err
	}
	// ports
	logPort := baseEnvManager.GetAvailableDeploymentPort(DefaultLogPort, tabs, nil)
	editorPort := baseEnvManager.GetAvailableDeploymentPort(DefaultEditorPort, tabs, []int{logPort})
	appProxyPort := baseEnvManager.GetAvailableDeploymentPort(DefaultAppProxyPort, tabs, []int{logPort, editorPort})
	// details
	details := &DeploymentDetails{}
	details.LogPort = strconv.Itoa(logPort)
	details.EditorPort = strconv.Itoa(editorPort)
	details.AppProxyPort = strconv.Itoa(appProxyPort)
	details.NodeHostName = NodeHostName
	details.EnvId = envId
	details.ClaimToken = claimToken
	details.LogUrl = fmt.Sprintf("%s://%s-%s.%s", NodeHostProtocol, "$sessionId", details.LogPort, details.NodeHostName)
	details.EditorUrl = fmt.Sprintf("%s://%s-%s.%s", NodeHostProtocol, "$sessionId", details.EditorPort, details.NodeHostName)
	for _, tab := range *tabs {
		tab.Url = fmt.Sprintf("%s://%s-%s-%d.%s%s", NodeHostProtocol, "$sessionId", details.AppProxyPort, tab.Port, details.NodeHostName, tab.Path)
	}
	details.Tabs = tabs
	return details, nil
}

func (baseEnvManager *BaseKubeEnvManager) GetDeploymentYaml(session *Session, template string, details *DeploymentDetails, detailsString string, minienvVersion string, nodeNameOverride string, nodeHostProtocol string, storageDriver string, repo *DeploymentRepo, envVars map[string]string) (string) {
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
	deployment := template
	deployment = strings.Replace(deployment, VarMinienvVersion, minienvVersion, -1)
	deployment = strings.Replace(deployment, VarMinienvNodeNameOverride, nodeNameOverride, -1)
	deployment = strings.Replace(deployment, VarMinienvNodeHostProtocol, nodeHostProtocol, -1)
	deployment = strings.Replace(deployment, VarAllowOrigin, allowOrigin, -1)
	deployment = strings.Replace(deployment, VarStorageDriver, storageDriver, -1)
	deployment = strings.Replace(deployment, VarGitRepoWithCreds, getUrlWithCredentials(repo.Repo, repo.Username, repo.Password), -1) // make sure this replace is done before gitRepo
	deployment = strings.Replace(deployment, VarGitRepo, repo.Repo, -1)
	deployment = strings.Replace(deployment, VarGitBranch, repo.Branch, -1)
	deployment = strings.Replace(deployment, VarAppProxyPort, details.AppProxyPort, -1)
	deployment = strings.Replace(deployment, VarLogPort, details.LogPort, -1)
	deployment = strings.Replace(deployment, VarEditorPort, details.EditorPort, -1)
	deployment = strings.Replace(deployment, VarDeploymentName, getEnvDeploymentName(details.EnvId), -1)
	deployment = strings.Replace(deployment, VarAppLabel, getEnvAppLabel(details.EnvId, details.ClaimToken), -1)
	deployment = strings.Replace(deployment, VarClaimToken, details.ClaimToken, -1)
	deployment = strings.Replace(deployment, VarEnvDetails, detailsString, -1)
	deployment = strings.Replace(deployment, VarEnvVars, envVarsYaml, -1)
	deployment = strings.Replace(deployment, VarPvcName, getPersistentVolumeClaimName(details.EnvId), -1)
	return deployment
}

func (baseEnvManager *BaseKubeEnvManager) GetServiceYaml(_ *Session, template string, details *DeploymentDetails) (string) {
	service := template
	service = strings.Replace(service, VarServiceName, getEnvServiceName(details.EnvId, details.ClaimToken), -1)
	service = strings.Replace(service, VarAppLabel, getEnvAppLabel(details.EnvId, details.ClaimToken), -1)
	service = strings.Replace(service, VarLogPort, details.LogPort, -1)
	service = strings.Replace(service, VarEditorPort, details.EditorPort, -1)
	service = strings.Replace(service, VarAppProxyPort, details.AppProxyPort, -1)
	return service
}

func (baseEnvManager *BaseKubeEnvManager) GetPersistentVolumeYaml(template string, envId string, storageSize string) (string) {
	pv := template
	pv = strings.Replace(pv, VarPvSize, storageSize, -1)
	pv = strings.Replace(pv, VarPvName, getPersistentVolumeName(envId), -1)
	pv = strings.Replace(pv, VarPvPath, getPersistentVolumePath(envId), -1)
	return pv
}

func (baseEnvManager *BaseKubeEnvManager) GetPersistentVolumeClaimYaml(template string, envId string, storageSize string, storageClass string) (string) {
	pvc := template
	pvc = strings.Replace(pvc, VarPvSize, storageSize, -1)
	pvc = strings.Replace(pvc, VarPvcName, getPersistentVolumeClaimName(envId), -1)
	pvc = strings.Replace(pvc, VarPvcStorageClass, storageClass, -1)
	return pvc
}

func (baseEnvManager *BaseKubeEnvManager) SerializeDeploymentDetails(details *DeploymentDetails) (string) {
	b, err := json.Marshal(details)
	if err != nil {
		return ""
	}
	s := strings.Replace(string(b), "\"", "\\\"", -1)
	return s
}

func (baseEnvManager *BaseKubeEnvManager) DeserializeDeploymentDetails(detailsStr string) (*DeploymentDetails) {
	detailsStr = strings.Replace(detailsStr, "\\\"", "\"", -1)
	var deploymentDetails DeploymentDetails
	err := json.Unmarshal([]byte(detailsStr), &deploymentDetails)
	if err != nil {
		return nil
	} else {
		return &deploymentDetails
	}
}