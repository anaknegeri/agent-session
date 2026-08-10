package entities

type Project struct {
	ID        string    `json:"id" db:"id"`
	Name      string    `json:"name" db:"name"`
	Path      string    `json:"path" db:"path"`
	CreatedAt Timestamp `json:"created_at" db:"created_at"`
}
