package contextkeys

type ContextKey string

const (
	EmailIDKey          ContextKey = "email_id"
	RequestIDKey        ContextKey = "request_id"
	TrackM1RequestIDKey ContextKey = "track_m1_request_id"
	UserIDKey           ContextKey = "userID"
	EmailKey            ContextKey = "email"
	SessionIDKey        ContextKey = "session_id"
	FamilyIDKey         ContextKey = "family_id"
	CFRayKey            ContextKey = "cf_ray"
	KeysInitializedKey  ContextKey = "keys_initialized"
)
