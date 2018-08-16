package minienv

import (
	"strings"
	"log"
	"net/http"
	"io/ioutil"
	"gopkg.in/yaml.v2"
	"fmt"
	"encoding/json"
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
	GetDeploymentTabsFromDockerCompose(repo *DeploymentRepo) (*[]*DeploymentTab, error)
	GetDeploymentDetails(envId string, claimToken string, repo *DeploymentRepo) (*DeploymentDetails, error)
	GetDeploymentYaml(envId string, claimToken string, minienvVersion string, nodeNameOverride string, nodeHostProtocol string, storageDriver string, repo *DeploymentRepo, details *DeploymentDetails, envVars map[string]string) (string)
	GetServiceYaml(envId string, claimToken string, details *DeploymentDetails) (string)
	GetPersistentVolumeYaml(envId string) (string)
	GetPersistentVolumeClaimYaml(envId string) (string)
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

func (envManager *BaseKubeEnvManager) GetDeploymentTabsFromDockerCompose(repo *DeploymentRepo) (*[]*DeploymentTab, error) {
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

func (envManager *BaseKubeEnvManager) GetDeploymentDetails(envId string, claimToken string, repo *DeploymentRepo) (*DeploymentDetails, error) {
	tabs, err := envManager.GetDeploymentTabsFromDockerCompose(repo)
	if err != nil {
		return nil, err
	}
	// ports
	details := &DeploymentDetails{}
	details.LogPort = DefaultLogPort
	details.EditorPort = DefaultEditorPort
	details.AppPort = DefaultProxyPort
	details.NodeHostName = NodeHostName
	details.EnvId = envId
	details.ClaimToken = claimToken
	details.LogUrl = fmt.Sprintf("%s://%s-%s.%s", NodeHostProtocol, "$sessionId", details.LogPort, details.NodeHostName)
	details.EditorUrl = fmt.Sprintf("%s://%s-%s.%s", NodeHostProtocol, "$sessionId", details.EditorPort, details.NodeHostName)
	for _, tab := range *tabs {
		tab.Url = fmt.Sprintf("%s://%s-%s-%d.%s%s", NodeHostProtocol, "$sessionId", details.AppPort, tab.Port, details.NodeHostName, tab.Path)
	}
	details.Tabs = tabs
	return details, nil
}

func (envManager *BaseKubeEnvManager) GetDeploymentYaml(envId string, claimToken string, minienvVersion string, nodeNameOverride string, nodeHostProtocol string, storageDriver string, repo *DeploymentRepo, details *DeploymentDetails, envVars map[string]string) (string) {
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
	deployment := envManager.GetDeploymentYamlTemplate()
	deployment = strings.Replace(deployment, VarMinienvVersion, minienvVersion, -1)
	deployment = strings.Replace(deployment, VarMinienvNodeNameOverride, nodeNameOverride, -1)
	deployment = strings.Replace(deployment, VarMinienvNodeHostProtocol, nodeHostProtocol, -1)
	deployment = strings.Replace(deployment, VarAllowOrigin, allowOrigin, -1)
	deployment = strings.Replace(deployment, VarStorageDriver, storageDriver, -1)
	deployment = strings.Replace(deployment, VarGitRepoWithCreds, getUrlWithCredentials(repo.Repo, repo.Username, repo.Password), -1) // make sure this replace is done before gitRepo
	deployment = strings.Replace(deployment, VarGitRepo, repo.Repo, -1)
	deployment = strings.Replace(deployment, VarGitBranch, repo.Branch, -1)
	deployment = strings.Replace(deployment, VarProxyPort, details.AppPort, -1)
	deployment = strings.Replace(deployment, VarLogPort, details.LogPort, -1)
	deployment = strings.Replace(deployment, VarEditorPort, details.EditorPort, -1)
	deployment = strings.Replace(deployment, VarDeploymentName, getEnvDeploymentName(envId), -1)
	deployment = strings.Replace(deployment, VarAppLabel, getEnvAppLabel(envId, claimToken), -1)
	deployment = strings.Replace(deployment, VarClaimToken, claimToken, -1)
	deployment = strings.Replace(deployment, VarEnvDetails, envManager.SerializeDeploymentDetails(details), -1)
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

func (envManager *BaseKubeEnvManager) SerializeDeploymentDetails(details *DeploymentDetails) (string) {
	b, err := json.Marshal(details)
	if err != nil {
		return ""
	}
	s := strings.Replace(string(b), "\"", "\\\"", -1)
	return s
}

func (envManager *BaseKubeEnvManager) DeserializeDeploymentDetails(detailsStr string) (*DeploymentDetails) {
	detailsStr = strings.Replace(detailsStr, "\\\"", "\"", -1)
	var deploymentDetails DeploymentDetails
	err := json.Unmarshal([]byte(detailsStr), &deploymentDetails)
	if err != nil {
		return nil
	} else {
		return &deploymentDetails
	}
}