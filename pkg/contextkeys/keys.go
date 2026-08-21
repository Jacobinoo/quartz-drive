package contextkeys

type ContextKey string

const (
	RequestIDKey ContextKey = "request_id"
	UserIDKey    ContextKey = "userID"
	EmailKey     ContextKey = "email"
)
