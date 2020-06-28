package message

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"github.com/bitmaelum/bitmaelum-server/core"
	"github.com/gabriel-vasile/mimetype"
	"github.com/google/uuid"
	"io"
	"os"
	"time"
)

type BlockType struct {
	Id          string     `json:"id"`              // BLock identifier UUID
	Type        string     `json:"type"`            // Type of the block. Can be anything message readers can parse.
	Size        uint64     `json:"size"`            // Size of the block in bytes
	Encoding    string     `json:"encoding"`        // Encoding of the block in case it's encoded
	Compression string     `json:"compression"`     // Compression used
	Checksum    []Checksum `json:"checksum"`        // Checksums of the block
	Reader      io.Reader  `json:"content"`         // Reader of the block data
	Key         []byte     `json:"key"`             // Key for decryption
	Iv          []byte     `json:"iv"`              // IV for decryption
}

type AttachmentType struct {
	Id          string     `json:"id"`              // Attachment identifier UUID
	MimeType    string     `json:"mimetype"`        // Mimetype
	FileName    string     `json:"filename"`        // Filename
	Size        uint64     `json:"size"`            // Size of the attachment in bytes
	Compression string     `json:"compression"`     // Compression used
	Checksum    []Checksum `json:"checksum"`        // Checksums of the data
	Reader      io.Reader  `json:"content"`         // Reader to the attachment data
	Key         []byte     `json:"key"`             // Key for decryption
	Iv          []byte     `json:"iv"`              // IV for decryption
}

type Catalog struct {
	From struct {
		Address      string           `json:"address"`              // BitMaelum address of the sender
		Name         string           `json:"name"`                 // Name of the sender
		Organisation string           `json:"organisation"`         // Organisation of the sender
		ProofOfWork  core.ProofOfWork `json:"proof_of_work"`        // Sender's proof of work
		PublicKey    string           `json:"public_key"`           // Public key of the sender
	} `json:"from"`
	To struct {
		Address string `json:"address"`                             // Address of the recipient
		Name    string `json:"name"`                                // Name of the recipient
	} `json:"to"`
	CreatedAt time.Time `json:"created_at"`                         // Timestamp when the message was created
	ThreadId  string    `json:"thread_id"`                          // Thread ID (and parent ID) in case this message was send in a thread
	Subject   string    `json:"subject"`                            // Subject of the message
	Flags     []string  `json:"flags"`                              // Flags of the message
	Labels    []string  `json:"labels"`                             // Labels for this message

	Blocks      []BlockType      `json:"blocks"`                    // Message block info
	Attachments []AttachmentType `json:"attachments"`               // Message attachment info
}

type Attachment struct {
	Path   string           // LOCAL path of the attachment. Needed for things like os.Stat()
	Reader io.Reader        // Reader to the attachment file
}

type Block struct {
	Type   string           // Type of the block (text, html, default, mobile etc)
	Size   uint64           // Size of the block
	Reader io.Reader        // Reader to the block data
}

// Initialises a new catalog. This catalog has to be filled with more info, blocks and attachments
func NewCatalog(ai *core.AccountInfo) *Catalog {
	c := &Catalog{}

	c.CreatedAt = time.Now()

	c.From.Address = ai.Address
	c.From.Name = ai.Name
	c.From.Organisation = ai.Organisation
	c.From.ProofOfWork.Bits = ai.Pow.Bits
	c.From.ProofOfWork.Proof = ai.Pow.Proof
	c.From.PublicKey = string(ai.PubKey)

	return c
}

// Add a block to a catalog
func (c *Catalog) AddBlock(entry Block) error {
	id, err := uuid.NewRandom()
	if err != nil {
		return err
	}

	var reader io.Reader = entry.Reader
	var compression = ""

	// Very arbitrary size on when we should compress output first
	if entry.Size >= 1024 {
		reader = core.ZlibCompress(entry.Reader)
		compression = "zlib"
	}

	// Generate key iv for this block
	iv, key, err := generateIvAndKey()
	if err != nil {
		return err
	}

	// Wrap reader with encryption reader
	reader,err = getAesReader(iv, key, reader)
	if err != nil {
		return err
	}

	bt := &BlockType{
		Id:          id.String(),
		Type:        entry.Type,
		Size:        entry.Size,
		Encoding:    "",
		Compression: compression,
		Checksum:    nil,
		Reader:      reader,
		Key:         key,
		Iv:          iv,
	}

	c.Blocks = append(c.Blocks, *bt)
	return nil
}

// Add an attachment to a catalog
func (c *Catalog) AddAttachment(entry Attachment) error {
	stats, err := os.Stat(entry.Path)
	if err != nil {
		return err
	}

	mime, err := mimetype.DetectReader(entry.Reader)
	if err != nil {
		return err
	}

	id, err := uuid.NewRandom()
	if err != nil {
		return err
	}

	var reader io.Reader = entry.Reader
	var compression = ""

	// Very arbitrary size on when we should compress output first
	if stats.Size() >= 1024 {
		reader = core.ZlibCompress(entry.Reader)
		compression = "zlib"
	}

	// Generate Key and IV that we will use for encryption
	iv, key, err := generateIvAndKey()
	if err != nil {
		return err
	}

	// Wrap our reader with the encryption reader
	reader,err = getAesReader(iv, key, reader)
	if err != nil {
		return err
	}

	at := &AttachmentType{
		Id:          id.String(),
		MimeType:    mime.String(),
		FileName:    entry.Path,
		Size:        uint64(stats.Size()),
		Compression: compression,
		Reader:      reader,
		Checksum:    nil,   // To be filled in later
		Key:         key,
		Iv:          iv,
	}

	c.Attachments = append(c.Attachments, *at)
	return nil
}

func generateIvAndKey() ([]byte, []byte, error) {
	iv := make([]byte, 16)
	n, err := rand.Read(iv)
	if n != 16 || err != nil {
		return nil, nil, err
	}

	key := make([]byte, 32)
	n, err = rand.Read(key)
	if n != 32 || err != nil {
		return nil, nil, err
	}

	return iv, key, nil
}

func getAesReader(iv []byte, key []byte, r io.Reader) (io.Reader, error) {
 	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}

	stream := cipher.NewCFBDecrypter(block, iv)
	return &cipher.StreamReader{S: stream, R: r}, err
}
