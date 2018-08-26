package minienv

import (
	"io/ioutil"
	"log"
	"strconv"
	"time"

)

const StatusIdle = 0
const StatusProvisioning = 1
const StatusClaimed = 2
const StatusRunning = 3

const CheckEnvTimerSeconds = 15
const ExpireClaimNoActivitySeconds int64 = 30
const DefaultEnvExpirationSeconds int64 = 60
const DefaultBranch = "master"

var minienvVersion = "latest"
var sessionStore SessionStore

var kubeServiceToken string
var kubeServiceBaseUrl string
var kubeNamespace string
var nodeNameOverride string
var nodeHostProtocol string
var storageDriver string
var allowOrigin string
var whitelistRepos []*WhitelistRepo

func loadFile(fp string) string {
	b, err := ioutil.ReadFile(fp) // just pass the file name
	if err != nil {
		log.Fatalf("Cannot read file")
	}
	return string(b)
}

func initEnvironments(apiServer *ApiServer, envCount int) {
	log.Printf("Provisioning %d environments...\n", envCount)
	for i := 0; i < envCount; i++ {
		environment := &Environment{Id: strconv.Itoa(i + 1)}
		apiServer.Environments = append(apiServer.Environments, environment)
		// check if environment running
		getDeploymentResp, err := getEnvDeployment(environment.Id, kubeServiceToken, kubeServiceBaseUrl, kubeNamespace)
		running := false
		if err == nil && getDeploymentResp != nil {
			log.Printf("Loading running environment %s...\n", environment.Id)
			if getDeploymentResp.Spec != nil &&
				getDeploymentResp.Spec.Template != nil &&
				getDeploymentResp.Spec.Template.Metadata != nil &&
				getDeploymentResp.Spec.Template.Metadata.Annotations != nil &&
				getDeploymentResp.Spec.Template.Metadata.Annotations.Repo != "" &&
				getDeploymentResp.Spec.Template.Metadata.Annotations.RepoWithCreds != "" &&
				getDeploymentResp.Spec.Template.Metadata.Annotations.ClaimToken != "" &&
				getDeploymentResp.Spec.Template.Metadata.Annotations.EnvDetails != "" {
				log.Printf("Loading environment %s from deployment metadata.\n", environment.Id)
				running = true
				details  := apiServer.EnvManager.DeserializeDeploymentDetails(getDeploymentResp.Spec.Template.Metadata.Annotations.EnvDetails)
				environment.Status = StatusRunning
				environment.ClaimToken = getDeploymentResp.Spec.Template.Metadata.Annotations.ClaimToken
				environment.LastActivity = time.Now().Unix()
				environment.Repo = getDeploymentResp.Spec.Template.Metadata.Annotations.Repo
				environment.RepoWithCreds = getDeploymentResp.Spec.Template.Metadata.Annotations.RepoWithCreds
				environment.Branch = getDeploymentResp.Spec.Template.Metadata.Annotations.Branch
				environment.Details = details
			} else {
				log.Printf("Insufficient deployment metadata for environment %s.\n", environment.Id)
				deleteEnv(environment.Id, environment.ClaimToken, kubeServiceToken, kubeServiceBaseUrl, kubeNamespace)
			}
		}
		if ! running {
			log.Printf("Provisioning environment %s...\n", environment.Id)
			environment.Status = StatusProvisioning
			deployProvisioner(apiServer.EnvManager, minienvVersion, environment.Id, nodeNameOverride, storageDriver, kubeServiceToken, kubeServiceBaseUrl, kubeNamespace)
		}
	}
	// scale down, if necessary
	i := envCount
	for true {
		envId := strconv.Itoa(i + 1)
		pvcName := getPersistentVolumeClaimName(envId)
		response, _ := getPersistentVolumeClaim(pvcName, kubeServiceToken, kubeServiceBaseUrl, kubeNamespace)
		if response != nil {
			log.Printf("De-provisioning environment %s...\n", envId)
			// get the deployment in order to find the claim token
			getDeploymentResp, err := getEnvDeployment(envId, kubeServiceToken, kubeServiceBaseUrl, kubeNamespace)
			if err == nil &&
				getDeploymentResp != nil &&
				getDeploymentResp.Spec != nil &&
				getDeploymentResp.Spec.Template != nil &&
				getDeploymentResp.Spec.Template.Metadata != nil &&
				getDeploymentResp.Spec.Template.Metadata.Annotations != nil &&
				getDeploymentResp.Spec.Template.Metadata.Annotations.ClaimToken != "" {
				claimToken := getDeploymentResp.Spec.Template.Metadata.Annotations.ClaimToken
				deleteEnv(envId, claimToken, kubeServiceToken, kubeServiceBaseUrl, kubeNamespace)
			} else {
				// we still want to call deleteEnv to tear down pvs, etc
				deleteEnv(envId, "", kubeServiceToken, kubeServiceBaseUrl, kubeNamespace)
			}
			deleteProvisioner(envId, kubeServiceToken, kubeServiceBaseUrl, kubeNamespace)
			deletePersistentVolumeClaim(pvcName, kubeServiceToken, kubeServiceBaseUrl, kubeNamespace)
			if apiServer.EnvManager.UseHostPathPersistentVolumes() {
				pvName := getPersistentVolumeName(envId)
				deletePersistentVolume(pvName, kubeServiceToken, kubeServiceBaseUrl)
			}
			i++
		} else {
			break
		}
	}
	checkEnvironments(apiServer)
}

func startEnvironmentCheckTimer(apiServer *ApiServer) {
	timer := time.NewTimer(time.Second * time.Duration(CheckEnvTimerSeconds))
	go func() {
		<-timer.C
		checkEnvironments(apiServer)
	}()
}

func checkEnvironments(apiServer *ApiServer) {
	for i := 0; i < len(apiServer.Environments); i++ {
		environment := apiServer.Environments[i]
		log.Printf("Checking environment %s; current status=%d\n", environment.Id, environment.Status)
		if environment.Status == StatusProvisioning {
			running, err := isProvisionerRunning(environment.Id, kubeServiceToken, kubeServiceBaseUrl, kubeNamespace)
			if err != nil {
				log.Println("Error checking provisioner status.", err)
			} else if ! running {
				log.Printf("Environment %s provisioning complete.\n", environment.Id)
				environment.Status = StatusIdle
				deleteProvisioner(environment.Id, kubeServiceToken, kubeServiceBaseUrl, kubeNamespace)
			} else {
				log.Printf("Environment %s still provisioning...\n", environment.Id)
			}
		} else if environment.Status == StatusRunning {
			if time.Now().Unix() - environment.LastActivity > DefaultEnvExpirationSeconds {
				log.Printf("Environment %s no longer active.\n", environment.Id)
				claimToken := environment.ClaimToken
				environment.Status = StatusIdle
				environment.ClaimToken = ""
				environment.LastActivity = 0
				environment.Repo = ""
				environment.Branch = ""
				environment.Details = nil
				environment.Props = nil
				deleteEnv(environment.Id, claimToken, kubeServiceToken, kubeServiceBaseUrl, kubeNamespace)
				// re-provision
				log.Printf("Re-provisioning environment %s...\n", environment.Id)
				environment.Status = StatusProvisioning
				deployProvisioner(apiServer.EnvManager, minienvVersion, environment.Id, nodeNameOverride, storageDriver, kubeServiceToken, kubeServiceBaseUrl, kubeNamespace)
			} else {
				log.Printf("Checking if environment %s is still deployed...\n", environment.Id)
				deployed, err := isEnvDeployed(environment.Id, kubeServiceToken, kubeServiceBaseUrl, kubeNamespace)
				if err == nil && ! deployed {
					log.Printf("Environment %s no longer deployed.\n", environment.Id)
					environment.Status = StatusIdle
					environment.ClaimToken = ""
					environment.LastActivity = 0
					environment.Repo = ""
					environment.Branch = ""
					environment.Details = nil
					environment.Props = nil
				}
			}
		}  else if environment.Status == StatusClaimed {
			if time.Now().Unix() - environment.LastActivity > ExpireClaimNoActivitySeconds {
				log.Printf("Environment %s claim expired.\n", environment.Id)
				environment.Status = StatusIdle
				environment.ClaimToken = ""
				environment.LastActivity = 0
				environment.Repo = ""
				environment.Branch = ""
				environment.Details = nil
				environment.Props = nil
			}
		}
	}
	startEnvironmentCheckTimer(apiServer)
}