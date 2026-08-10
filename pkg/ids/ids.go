package ids

import (
	"fmt"
	"strings"

	"github.com/google/uuid"
)

// New returns a prefixed ID, e.g. New("sess") -> "sess_0f9a...".
func New(prefix string) string {
	return fmt.Sprintf("%s_%s", prefix, strings.ReplaceAll(uuid.NewString(), "-", "")[:16])
}
