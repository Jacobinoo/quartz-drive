package refresh

type RefreshResponse struct {
	Status             string `json:"status"`
	Token              string `json:"token"`
	CsrfToken          string `json:"csrfToken"`
	WrappedAccountKeys string `json:"wrappedAccountKeys"`
}
