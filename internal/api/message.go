package api

import (
	"fmt"
	"github.com/bitmaelum/bitmaelum-suite/internal/message"
	"github.com/bitmaelum/bitmaelum-suite/pkg/address"
	"io"
)

// Message is a standard structure that returns a message header + catalog
type Message struct {
	ID      string         `json:"id"`
	Header  message.Header `json:"h"`
	Catalog []byte         `json:"c"`
}

// GetMessage retrieves a message header + catalog from a message box
func (api *API) GetMessage(addr address.HashAddress, box, messageID string) (*Message, error) {
	in := &Message{}

	resp, statusCode, err := api.GetJSON(fmt.Sprintf("/account/%s/box/%s/message/%s", addr.String(), box, messageID), in)
	if err != nil {
		return nil, err
	}

	if statusCode < 200 || statusCode > 299 {
		return nil, getErrorFromResponse(resp)
	}

	return in, nil
}

// GetMessageBlock retrieves a message block
func (api *API) GetMessageBlock(addr address.HashAddress, box, messageID, blockID string) ([]byte, error) {
	body, statusCode, err := api.Get(fmt.Sprintf("/account/%s/box/%s/message/%s/block/%s", addr.String(), box, messageID, blockID))
	if err != nil {
		return nil, err
	}

	if statusCode < 200 || statusCode > 299 {
		return nil, errNoSuccess
	}

	return body, nil
}

// GetMessageAttachment retrieves a message attachment reader
func (api *API) GetMessageAttachment(addr address.HashAddress, box, messageID, attachmentID string) (io.Reader, error) {
	r, statusCode, err := api.GetReader(fmt.Sprintf("/account/%s/box/%s/message/%s/attachment/%s", addr.String(), box, messageID, attachmentID))
	if err != nil {
		return nil, err
	}

	if statusCode < 200 || statusCode > 299 {
		return nil, errNoSuccess
	}

	return r, nil
}
