package model

// Searchable records keep search.Text(SearchSource()) in their SearchText
// column (global search). Their BeforeSave hooks refresh it on Create/Save;
// updates of single columns must not touch the searchable fields.
type Searchable interface {
	SearchSource() []string
}
