package annotations

const (
	KeyDescription = "ksyncer/description"
	KeyInput       = "ksyncer/input"
	KeyOutput      = "ksyncer/output"
	KeyVisibility  = "ksyncer/visibility"
)

// RelevantKeys lists annotation keys that trigger resync.
var RelevantKeys = []string{KeyDescription, KeyVisibility, KeyInput, KeyOutput}
