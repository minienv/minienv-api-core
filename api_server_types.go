package minienv

type Environment struct {
	Id string
	Status int
	ClaimToken string
	LastActivity int64
	Repo string
	RepoWithCreds string
	Branch string
	Details *DeploymentDetails
	ExpirationSeconds int64
}

type WhitelistRepo struct {
	Name string `json:"name"`
	Url string `json:"url"`
	Branch string `json:"branch"`
}

type ClaimRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type ClaimResponse struct {
	ClaimGranted bool `json:"claimGranted"`
	ClaimToken string `json:"claimToken"`
	Message string `json:"message"`
}

type WhitelistResponse struct {
	Repos []*WhitelistRepo `json:"repos"`
}

type PingRequest struct {
	ClaimToken string `json:"claimToken"`
	GetEnvDetails bool `json:"getEnvDetails"`
}

type PingResponse struct {
	ClaimGranted bool `json:"claimGranted"`
	Up bool `json:"up"`
	Repo string `json:"repo"`
	Branch string `json:"branch"`
	EnvDetails *EnvUpResponse `json:"envDetails"`
}

type EnvInfoRequest struct {
	Repo string `json:"repo"`
	Branch string `json:"branch"`
	Username string `json:"username"`
	Password string `json:"password"`
}

type EnvInfoResponse struct {
}

type EnvUpRequest struct {
	ClaimToken string `json:"claimToken"`
	Repo string `json:"repo"`
	Branch string `json:"branch"`
	Username string `json:"username"`
	Password string `json:"password"`
	ExpirationSeconds int64 `json:"expirationSeconds"`
	EnvVars map[string]string `json:"envVars"`
}

type EnvUpResponse struct {
	LogUrl string        `json:"logUrl"`
	EditorUrl string     `json:"editorUrl"`
	Tabs []DeploymentTab `json:"tabs"`
}
