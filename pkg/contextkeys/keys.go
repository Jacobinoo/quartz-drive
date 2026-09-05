package contextkeys

type ContextKey string

const (
	EmailIDKey         ContextKey = "email_id"
	RequestIDKey       ContextKey = "request_id"
	UserIDKey          ContextKey = "userID"
	EmailKey           ContextKey = "email"
	SessionIDKey       ContextKey = "session_id"
	FamilyIDKey        ContextKey = "family_id"
	CFRayKey           ContextKey = "cf_ray"
	KeysInitializedKey ContextKey = "keys_initialized"
)
