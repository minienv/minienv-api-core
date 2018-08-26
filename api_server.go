package minienv

import (
	"errors"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

type ApiServer struct {
	EnvManager KubeEnvManager
	Environments []*Environment
}

func (apiServer *ApiServer) GetOrCreateSession(id string) *Session {
	var session *Session = nil
	if id != "" {
		session, _ = sessionStore.GetSession(id)
	}
	if session == nil {
		random, _ := uuid.NewRandom()
		id = strings.Replace(random.String(), "-", "", -1)
		session = &Session{Id: id}
		sessionStore.SetSession(id, session)
	}
	return session
}

func (apiServer *ApiServer) GetSession(id string) (*Session, error) {
	return sessionStore.GetSession(id)
}

func (apiServer *ApiServer) SetSession(id string, session *Session) (error) {
	return sessionStore.SetSession(id, session)
}

func (apiServer *ApiServer) AddCorsAndCacheHeadersThenServe(handler http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Access-Control-Allow-Origin", allowOrigin)
		w.Header().Add("Access-Control-Allow-Headers", "Content-Type")
		w.Header().Add("Access-Control-Allow-Headers", "Minienv-Session-Id")
		w.Header().Add("Cache-Control", "no-store, must-revalidate")
		w.Header().Add("Expires", "0")
		if r.Method == "OPTIONS" {
			return
		}
		handler(w, r)
	}
}

func (apiServer *ApiServer) Claim(request *ClaimRequest) (*ClaimResponse){
	var claimResponse = ClaimResponse{}
	var environment *Environment
	for _, element := range apiServer.Environments {
		if element.Status == StatusIdle {
			environment = element
			break
		}
	}
	if environment == nil {
		log.Println("Claim failed; no environments available.")
		claimResponse.ClaimGranted = false
		claimResponse.Message = "No environments available"
	} else {
		log.Printf("Claimed environment %s.\n", environment.Id)
		// ok, grant claim and create new environment
		claimToken, _ := uuid.NewRandom()
		claimTokenStr := strings.Replace(claimToken.String(), "-", "", -1)
		claimResponse.ClaimGranted = true
		claimResponse.ClaimToken = claimTokenStr
		// update environment
		environment.ClaimToken = claimResponse.ClaimToken
		environment.Status = StatusClaimed
		environment.LastActivity = time.Now().Unix()
	}
	return &claimResponse
}

func (apiServer *ApiServer) Whitelist() (*WhitelistResponse) {
	var whitelistResponse = WhitelistResponse{}
	whitelistResponse.Repos = whitelistRepos
	return &whitelistResponse
}

func (apiServer *ApiServer) Ping(pingRequest *PingRequest, session *Session) (*PingResponse, error) {
	var pingResponse = PingResponse{}
	var environment *Environment
	for _, element := range apiServer.Environments {
		if element.ClaimToken == pingRequest.ClaimToken {
			environment = element
			break
		}
	}
	if environment == nil {
		pingResponse.ClaimGranted = false
		pingResponse.Up = false
	} else {
		environment.LastActivity = time.Now().Unix()
		pingResponse.ClaimGranted = true
		pingResponse.Up = environment.Status == StatusRunning
		pingResponse.Repo = environment.Repo
		pingResponse.Branch = environment.Branch
		if pingResponse.Up && pingRequest.GetEnvDetails {
			// make sure to check if it is really running
			exists, err := isEnvDeployed(environment.Id, kubeServiceToken, kubeServiceBaseUrl, kubeNamespace)
			if err != nil {
				log.Println("Error querying Kubernetes: ", err)
				return nil, err
			}
			pingResponse.Up = exists
			if exists {
				pingResponse.EnvDetails = getEnvUpResponse(environment.Details, session)
			} else {
				environment.Status = StatusClaimed
				environment.Repo = ""
				environment.Branch = ""
				environment.Details = nil
				environment.Props = nil
			}
		}
	}
	return &pingResponse, nil
}

func (apiServer *ApiServer) Info(envInfoRequest *EnvInfoRequest, session *Session) (*EnvInfoResponse, error) {
	if envInfoRequest.Branch == "" {
		envInfoRequest.Branch = DefaultBranch
	}
	if whitelistRepos != nil {
		repoWhitelisted := false
		for _, element := range whitelistRepos {
			if envInfoRequest.Repo == element.Url && envInfoRequest.Branch == element.Branch {
				repoWhitelisted = true
				break
			}
		}
		if ! repoWhitelisted {
			log.Println("Info request failed; repo not whitelisted.")
			return nil, errors.New("requested repo not whitelisted")
		}
	}
	// create response
	var envInfoResponse = EnvInfoResponse{}
	return &envInfoResponse, nil
}

func (apiServer *ApiServer) Up(envUpRequest *EnvUpRequest, session *Session) (*EnvUpResponse, error) {
	if envUpRequest.Branch == "" {
		envUpRequest.Branch = DefaultBranch
	}
	var environment *Environment
	for _, element := range apiServer.Environments {
		if element.ClaimToken == envUpRequest.ClaimToken {
			environment = element
			break
		}
	}
	if environment == nil {
		log.Println("Up request failed; claim no longer valid.")
		return nil, errors.New("invalid claim token")
	} else {
		if whitelistRepos != nil {
			repoWhitelisted := false
			for _, element := range whitelistRepos {
				if envUpRequest.Repo == element.Url && envUpRequest.Branch == element.Branch {
					repoWhitelisted = true
					break
				}
			}
			if ! repoWhitelisted {
				log.Println("Up request failed; repo not whitelisted.")
				return nil, errors.New("requested repo not whitelisted")
			}
		}
		// create response
		var envUpResponse *EnvUpResponse
		log.Printf("Checking if deployment exists for env %s...\n", environment.Id)
		exists, err := isEnvDeployed(environment.Id, kubeServiceToken, kubeServiceBaseUrl, kubeNamespace)
		if err != nil {
			log.Printf("Error checking if deployment exists for env %s: %s\n", environment.Id, err)
			return nil, errors.New("unable to find deployment")
		} else if exists {
			log.Printf("Env deployed for claim %s.\n", environment.Id)
			// mw:commented out to allow re-deployment
			//if environment.Status == StatusRunning && strings.EqualFold(envUpRequest.Repo, environment.Repo) && strings.EqualFold(envUpRequest.Branch, environment.Branch) {
			//	log.Println("Returning existing environment details...")
			//	envUpResponse = getEnvUpResponse(environment.Details, session)
			//}
		}
		if envUpResponse == nil {
			log.Printf("Creating new deployment...")
			// change status to claimed, so the scheduler doesn't think it has stopped when the old repo is shutdown
			environment.Status = StatusClaimed
			repo := &DeploymentRepo{
				Repo: envUpRequest.Repo,
				Branch: envUpRequest.Branch,
				Username: envUpRequest.Username,
				Password: envUpRequest.Password,
			}
			details, err := deployEnv(session, apiServer.EnvManager, minienvVersion, environment.Id, environment.ClaimToken, nodeNameOverride, nodeHostProtocol, repo, envUpRequest.EnvVars, storageDriver, kubeServiceToken, kubeServiceBaseUrl, kubeNamespace)
			if err != nil || details == nil {
				log.Print("Error creating deployment: ", err)
				return nil, errors.New("error creating deployment")
			} else {
				envUpResponse = getEnvUpResponse(details, session)
				environment.Status = StatusRunning
				environment.Repo = envUpRequest.Repo
				environment.Branch = envUpRequest.Branch
				environment.Details = details
				if envUpRequest.ExpirationSeconds >= 0 {
					environment.ExpirationSeconds = envUpRequest.ExpirationSeconds
				} else {
					environment.ExpirationSeconds = DefaultEnvExpirationSeconds
				}
			}
		}
		return envUpResponse, nil
	}
}

func getEnvUpResponse(details *DeploymentDetails, session *Session) (*EnvUpResponse) {
	sessionIdStr := ""
	if session != nil {
		sessionIdStr = session.Id
		session.EnvId = details.EnvId
		session.EnvServiceName = getEnvServiceName(details.EnvId, details.ClaimToken)
		sessionStore.SetSession(session.Id, session)
	}
	sessionIdStr = strconv.FormatInt(int64(time.Now().Unix()), 16) + "-" + sessionIdStr
	envUpResponse := &EnvUpResponse{}
	envUpResponse.LogUrl = strings.Replace(details.LogUrl, "$sessionId", sessionIdStr, -1)
	envUpResponse.EditorUrl = strings.Replace(details.EditorUrl, "$sessionId", sessionIdStr, -1)
	envUpResponse.Tabs = []DeploymentTab{}
	if details.Tabs != nil {
		for _, element := range *details.Tabs {
			tab := DeploymentTab{
				Port: element.Port,
				Url: strings.Replace(element.Url, "$sessionId", sessionIdStr, -1),
				Name: element.Name,
				Path: element.Path,
			}
			envUpResponse.Tabs = append(envUpResponse.Tabs, tab)
		}
	}
	return envUpResponse
}

func NewBaseKubeEnvManager() (*BaseKubeEnvManager) {
	envManager := &BaseKubeEnvManager{}
	envManager.ProvisionerJobYamlTemplate = loadFile("./provisioner-job.yml")
	envManager.ProvisionVolumeSize = os.Getenv("MINIENV_PROVISION_VOLUME_SIZE")
	envManager.ProvisionImages = os.Getenv("MINIENV_PROVISION_IMAGES")
	envManager.PersistentVolumeStorageClass = os.Getenv("MINIENV_VOLUME_STORAGE_CLASS")
	if envManager.PersistentVolumeStorageClass == "" {
		envManager.PersistentVolumeHostPath = true
		envManager.PersistentVolumeYamlTemplate = loadFile("./env-pv-host-path.yml")
		envManager.PersistentVolumeClaimYamlTemplate = loadFile("./env-pvc-host-path.yml")
	} else {
		envManager.PersistentVolumeHostPath = false
		envManager.PersistentVolumeClaimYamlTemplate = loadFile("./env-pvc-storage-class.yml")
	}
	envManager.ServiceYamlTemplate = loadFile("./env-service.yml")
	envManager.DeploymentYamlTemplate = loadFile("./env-deployment.yml")
	return envManager
}

func (apiServer *ApiServer) Init() {
	if apiServer.EnvManager == nil {
		apiServer.EnvManager = NewBaseKubeEnvManager()
	}
	minienvVersion = os.Getenv("MINIENV_VERSION")
	redisAddress := os.Getenv("MINIENV_REDIS_ADDRESS")
	redisPassword := os.Getenv("MINIENV_REDIS_PASSWORD")
	redisDb := os.Getenv("MINIENV_REDIS_DB")
	if redisAddress != "" {
		redisSessionStore, err := NewRedisSessionStore(redisAddress, redisPassword, redisDb)
		if err != nil {
			sessionStore = nil
		} else {
			sessionStore = redisSessionStore
		}
	}
	if sessionStore == nil {
		sessionStore = NewInMemorySessionStore()
	}
	kubeServiceProtocol := os.Getenv("KUBERNETES_SERVICE_PROTOCOL")
	kubeServiceHost := os.Getenv("KUBERNETES_SERVICE_HOST")
	kubeServicePort := os.Getenv("KUBERNETES_SERVICE_PORT")
	kubeServiceTokenPathEnv := os.Getenv("KUBERNETES_TOKEN_PATH")
	if len(kubeServiceTokenPathEnv) > 0 {
		kubeServiceToken = loadFile(kubeServiceTokenPathEnv)
	} else {
		kubeServiceToken = ""
	}
	if len(kubeServiceProtocol) > 0 {
		kubeServiceBaseUrl = kubeServiceProtocol
	} else {
		kubeServiceBaseUrl = "https://"
	}
	kubeServiceBaseUrl += kubeServiceHost
	kubeServiceBaseUrl += ":"
	kubeServiceBaseUrl += kubeServicePort
	kubeNamespace = os.Getenv("MINIENV_NAMESPACE")
	if kubeNamespace == "" {
		kubeNamespace = "default"
	}
	nodeNameOverride = os.Getenv("MINIENV_NODE_NAME_OVERRIDE")
	nodeHostProtocol = os.Getenv("MINIENV_NODE_HOST_PROTOCOL")
	storageDriver = os.Getenv("MINIENV_STORAGE_DRIVER")
	if storageDriver == "" {
		storageDriver = "aufs"
	}
	allowOrigin = os.Getenv("MINIENV_ALLOW_ORIGIN")
	envCount := 1
	if i, err := strconv.Atoi(os.Getenv("MINIENV_PROVISION_COUNT")); err == nil {
		envCount = i
	}
	whitelistReposStr := os.Getenv("MINIENV_REPO_WHITELIST")
	if whitelistReposStr == "" {
		whitelistRepos = nil
	} else {
		whitelistRepoStrs := strings.Split(whitelistReposStr, ",")
		if len(whitelistRepoStrs) <= 0 {
			whitelistRepos = nil
		} else {
			whitelistRepos = []*WhitelistRepo{}
			var name string
			var url string
			var branch string
			for _, element := range whitelistRepoStrs {
				elementStrs := strings.Split(element, "|")
				if len(elementStrs) >= 2 {
					name = elementStrs[0]
					url = elementStrs[1]
					if len(elementStrs) == 3 {
						branch = elementStrs[2]
					} else {
						branch = DefaultBranch
					}
				} else {
					name = element
					url = element
					branch = DefaultBranch
				}
				whitelistRepo := &WhitelistRepo{Name: name, Url: url, Branch: branch}
				whitelistRepos = append(whitelistRepos, whitelistRepo)
			}
		}
	}
	initEnvironments(apiServer, envCount)
}
