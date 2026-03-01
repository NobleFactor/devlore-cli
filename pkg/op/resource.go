package op

// Resource represents a logical data item identified by a URI and tracked across distributed nodes.
//
// It serves as a reference entry in the ResourceLedger with origin tracking information.
type Resource struct {
	URI          string // logical address of the resource (e.g., a file URL)
	ID           string // unique identifier in the flat ResourceLedger
	OriginNodeID string // ID of the node that created this resource
}
