package minienv

import (
	"fmt"
	"log"
	"os"
	"reflect"
	"strconv"
	"strings"

)

var NodeHostName = os.Getenv("MINIENV_NODE_HOST_NAME")
var NodeHostProtocol = os.Getenv("MINIENV_NODE_HOST_PROTOCOL")

var VarMinienvVersion = "$minienvVersion"
var VarMinienvNodeNameOverride = "$nodeNameOverride"
var VarMinienvNodeHostProtocol = "$nodeHostProtocol"
var VarAllowOrigin = "$allowOrigin"
var VarStorageDriver = "$storageDriver"
var VarGitRepo = "$gitRepo"
var VarGitRepoWithCreds = "$gitRepoWithCreds"
var VarGitBranch = "$gitBranch"
var VarProxyPort = "$proxyPort"
var VarLogPort = "$logPort"
var VarEditorPort = "$editorPort"
var VarPvName = "$pvName"
var VarPvSize = "$pvSize"
var VarPvPath = "$pvPath"
var VarPvcName = "$pvcName"
var VarPvcStorageClass = "$pvcStorageClass"
var VarServiceName = "$serviceName"
var VarDeploymentName = "$deploymentName"
var VarAppLabel = "$appLabel"
var VarClaimToken = "$claimToken"
var VarEnvDetails = "$envDetails"
var VarEnvVars = "$envVars"

var DefaultLogPort = "8001"
var DefaultEditorPort = "8002"
var DefaultProxyPort = "8003"


type DeploymentTab struct {
	Port int `json:"port"`
	Url string `json:"url"`
	Hide bool   `json:"hide"`
	Name string `json:"name"`
	Path string `json:"path"`
}

type DeploymentRepo struct {
	Repo string
	Branch string
	Username string
	Password string
}

type DeploymentDetails struct {
	NodeHostName string
	EnvId string
	ClaimToken string
	LogPort string
	LogUrl string
	EditorPort string
	EditorUrl string
	AppPort string
	Tabs *[]*DeploymentTab
	Props *map[string]interface{}
}

func getEnvDeployment(envId string, kubeServiceToken string, kubeServiceBaseUrl string, kubeNamespace string) (*GetDeploymentResponse, error) {
	return getDeployment(getEnvDeploymentName(envId), kubeServiceToken, kubeServiceBaseUrl, kubeNamespace)
}

func isEnvDeployed(envId string, kubeServiceToken string, kubeServiceBaseUrl string, kubeNamespace string) (bool, error) {
	getDeploymentResp, err := getDeployment(getEnvDeploymentName(envId), kubeServiceToken, kubeServiceBaseUrl, kubeNamespace)
	if err != nil {
		return false, err
	} else {
		return getDeploymentResp != nil, nil
	}
}

func deleteEnv(envId string, claimToken string, kubeServiceToken string, kubeServiceBaseUrl string, kubeNamespace string) {
	log.Printf("Deleting env %s...\n", envId)
	deploymentName := getEnvDeploymentName(envId)
	appLabel := getEnvAppLabel(envId, claimToken)
	serviceName := getEnvServiceName(envId, claimToken)
	_, _ = deleteDeployment(deploymentName, kubeServiceToken, kubeServiceBaseUrl, kubeNamespace)
	_, _ = deleteReplicaSet(appLabel, kubeServiceToken, kubeServiceBaseUrl, kubeNamespace)
	_, _ = deleteService(serviceName, kubeServiceToken, kubeServiceBaseUrl, kubeNamespace)
	_, _ = waitForPodTermination(appLabel, kubeServiceToken, kubeServiceBaseUrl, kubeNamespace)
}

func getUrlWithCredentials(url string, username string, password string) (string) {
	if username != "" && password != "" {
		url = strings.Replace(url, "https://", fmt.Sprintf("https://%s:%s@", username, password), 1)
		url = strings.Replace(url, "http://", fmt.Sprintf("http://%s:%s@", username, password), 1)
	}
	return url
}

func getDownloadUrl(path string, gitRepo string, gitBranch string, gitUsername string, gitPassword string) (string) {
	url := fmt.Sprintf("%s/%s/%s", gitRepo, gitBranch, path)
	url = strings.Replace(url, "github.com", "raw.githubusercontent.com", 1)
	url = getUrlWithCredentials(url, gitUsername, gitPassword)
	return url
}

func deployEnv(envManager KubeEnvManager, minienvVersion string, envId string, claimToken string, nodeNameOverride string, nodeHostProtocol string, repo *DeploymentRepo, envVars map[string]string, storageDriver string, kubeServiceToken string, kubeServiceBaseUrl string, kubeNamespace string) (*DeploymentDetails, error) {
	// delete env, if it exists
	deleteEnv(envId, claimToken, kubeServiceToken, kubeServiceBaseUrl, kubeNamespace)
	// get deployment details
	details, _ := envManager.GetDeploymentDetails(envId, claimToken, repo)
	// create persistent volume if using host paths
	if envManager.UseHostPathPersistentVolumes() {
		pvResponse, err := getPersistentVolume(getPersistentVolumeName(envId), kubeServiceToken, kubeServiceBaseUrl)
		if err != nil {
			log.Println("Error getting persistent volume: ", err)
			return nil, err
		} else if pvResponse == nil {
			_, err = savePersistentVolume(envManager.GetPersistentVolumeYaml(envId), kubeServiceToken, kubeServiceBaseUrl)
			if err != nil {
				log.Println("Error saving persistent volume: ", err)
				return nil, err
			}
		}
	}
	// create persistent volume claim, if not exists
	pvcResponse, err := getPersistentVolumeClaim(getPersistentVolumeClaimName(envId), kubeServiceToken, kubeServiceBaseUrl, kubeNamespace)
	if err != nil {
		log.Println("Error getting persistent volume claim: ", err)
		return nil, err
	} else if pvcResponse == nil {
		_, err = savePersistentVolumeClaim(envManager.GetPersistentVolumeClaimYaml(envId), kubeServiceToken, kubeServiceBaseUrl, kubeNamespace)
		if err != nil {
			log.Println("Error saving persistent volume claim: ", err)
			return nil, err
		}
	}
	// create the service first - we need the ports to serialize the details with the deployment
	service := envManager.GetServiceYaml(envId, claimToken, details)
	_, err = saveService(service, kubeServiceToken, kubeServiceBaseUrl, kubeNamespace)
	if err != nil {
		log.Println("Error saving service: ", err)
		return nil, err
	}
	// save deployment
	deployment := envManager.GetDeploymentYaml(envId, claimToken, minienvVersion, nodeNameOverride, nodeHostProtocol, storageDriver, repo, details, envVars)
	_, err = saveDeployment(deployment, kubeServiceToken, kubeServiceBaseUrl, kubeNamespace)
	if err != nil {
		log.Println("Error saving deployment: ", err)
		return nil, err
	}
	// return
	return details, nil
}



func populateTabs(v interface{}, tabs *[]*DeploymentTab, parent string) {
	typ := reflect.TypeOf(v).Kind()
	if typ == reflect.String {
		if parent == "ports" {
			portString := strings.SplitN(v.(string), ":", 2)[0]
			port, err := strconv.Atoi(portString)
			if err == nil {
				tab := &DeploymentTab{}
				tab.Port = port
				tab.Name = strconv.Itoa(port)
				*tabs = append(*tabs, tab)
			}
		}
	} else if typ == reflect.Slice {
		populateTabsSlice(v.([]interface{}), tabs, parent)
	} else if typ == reflect.Map {
		populateTabsMap(v.(map[interface{}]interface{}), tabs)
	}
}

func populateTabsMap(m map[interface{}]interface{}, tabs *[]*DeploymentTab) {
	for k, v := range m {
		populateTabs(v, tabs, strings.ToLower(k.(string)))
	}
}

func populateTabsSlice(slc []interface{}, tabs *[]*DeploymentTab, parent string) {
	for _, v := range slc {
		populateTabs(v, tabs, parent)
	}
}

func getPersistentVolumeName(envId string) string {
	return strings.ToLower(fmt.Sprintf("minienv-env-%s-pv", envId))
}

func getPersistentVolumePath(envId string) string {
	return strings.ToLower(fmt.Sprintf("/minienv-env-%s", envId))
}

func getPersistentVolumeClaimName(envId string) string {
	return strings.ToLower(fmt.Sprintf("env-%s-pvc", envId))
}

func getEnvDeploymentName(envId string) string {
	return strings.ToLower(fmt.Sprintf("env-%s-deployment", envId))
}

// service name and app label are based on claim token
// this way users won't mistakenly get access to services and deployments that they shouldn't have access to
func getEnvServiceName(envId string, claimToken string) string {
	return strings.ToLower(fmt.Sprintf("env-%s-service-%s", envId, claimToken))
}

func getEnvAppLabel(envId string, claimToken string) string {
	return strings.ToLower(fmt.Sprintf("env-%s-app-%s", envId, claimToken))
}