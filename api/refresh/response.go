package refresh

type RefreshResponse struct {
	Status            string `json:"status"`
	Token             string `json:"token"`
	CsrfToken         string `json:"csrfToken"`
	SessionPrivateKey string `json:"sessionPrivateKey"`
}
