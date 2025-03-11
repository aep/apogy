package apogy

// BookVal represents a generated struct
type BookVal struct {
	Author  string           `json:"author"`
	Isbn    *string          `json:"isbn,omitempty"`
	Name    string           `json:"name"`
	Reviews []BookValReviews `json:"reviews,omitempty"`
}

// BookValReviews represents a generated struct
type BookValReviews struct {
	Author string `json:"author"`
	Text   string `json:"text"`
}
