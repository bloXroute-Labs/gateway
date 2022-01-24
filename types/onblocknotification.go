package types

// OnBlockNotification - represents the result of an RPC call on published block
type OnBlockNotification struct {
	Name        string `json:"name,omitempty"`
	Response    string `json:"response,omitempty"`
	BlockHeight string `json:"block_height,omitempty"`
	Tag         string `json:"tag,omitempty"`
	hash        string
}

// NewOnBlockNotification returns a new OnBlockNotification
func NewOnBlockNotification(name string, response string, blockHeight string, tag string, hash string) *OnBlockNotification {
	return &OnBlockNotification{
		Name:        name,
		Response:    response,
		BlockHeight: blockHeight,
		Tag:         tag,
		hash:        hash,
	}
}

// WithFields -
func (n *OnBlockNotification) WithFields(fields []string) Notification {
	onBlockNotification := OnBlockNotification{}
	for _, param := range fields {
		switch param {
		case "name":
			onBlockNotification.Name = n.Name
		case "response":
			onBlockNotification.Response = n.Response
		case "block_height":
			onBlockNotification.BlockHeight = n.BlockHeight
		case "tag":
			onBlockNotification.Tag = n.Tag
		}
	}
	return &onBlockNotification
}

// Filters -
func (n *OnBlockNotification) Filters(filters []string) map[string]interface{} {
	return nil
}

// LocalRegion -
func (n *OnBlockNotification) LocalRegion() bool {
	return false
}

// GetHash -
func (n *OnBlockNotification) GetHash() string {
	return n.hash
}

// NotificationType - feed name
func (n *OnBlockNotification) NotificationType() FeedType {
	return OnBlockFeed
}
