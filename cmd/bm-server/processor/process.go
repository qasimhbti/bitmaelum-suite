package processor

import (
	"github.com/bitmaelum/bitmaelum-suite/internal/account"
	"github.com/bitmaelum/bitmaelum-suite/internal/api"
	"github.com/bitmaelum/bitmaelum-suite/internal/config"
	"github.com/bitmaelum/bitmaelum-suite/internal/container"
	"github.com/bitmaelum/bitmaelum-suite/internal/message"
	"github.com/bitmaelum/bitmaelum-suite/internal/resolve"
	"github.com/bitmaelum/bitmaelum-suite/pkg/address"
	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
	"io/ioutil"
	"os"
)

// ProcessMessage will process a message found in the processing queue.
//   * If it's a local address, it will be moved to the local mailbox
//   * If it's a remote address, it will be send to the remote mail server
//   * If things fail, it will be moved to the retry queue, where it will be moved to processed queue later
func ProcessMessage(msgID string) {
	logrus.Debugf("processing message %s", msgID)

	// Set the message in the scoreboard, so we know this message is being processed.
	AddToScoreboard(message.SectionProcessing, msgID)
	defer func() {
		RemoveFromScoreboard(message.SectionProcessing, msgID)
	}()

	// Check header and get recipient
	header, err := message.GetMessageHeader(message.SectionProcessing, msgID)
	if err != nil {
		// cannot read header.. Let's move to retry queue
		logrus.Warnf("cannot find or read header for message %s. Retrying.", msgID)
		MoveToRetryQueue(msgID)
		return
	}

	rs := container.GetResolveService()
	res, err := rs.Resolve(header.To.Addr)
	if err != nil {
		logrus.Warnf("cannot resolve address %s for message %s. Retrying.", header.To.Addr, msgID)
		MoveToRetryQueue(msgID)
		return
	}

	// Local addresses don't need to be send. They are treated locally
	ar := container.GetAccountRepo()
	if ar.Exists(header.To.Addr) {
		// probably move the message to the incoming queue
		// Do stuff locally
		logrus.Debugf("Message %s can be transferred locally to %s", msgID, res.Hash)

		err := deliverLocal(res, msgID)
		if err != nil {
			logrus.Warnf("cannot deliver message %s locally to %s. Retrying.", msgID, header.To.Addr)
			MoveToRetryQueue(msgID)
		}
		return
	}

	// Otherwise, send to outgoing server
	logrus.Debugf("Message %s is remote, transferring to %s", msgID, res.Server)
	err = deliverRemote(header, res, msgID)
	if err != nil {
		logrus.Warnf("cannot deliver message %s remotely to %s. Retrying.", msgID, header.To.Addr)
		MoveToRetryQueue(msgID)
	}
}

// deliverLocal moves a message to a local mailbox. This is an easy process as it only needs to move
// the message to another directory.
func deliverLocal(info *resolve.Info, msgID string) error {
	// Deliver mail to local user's inbox
	ar := container.GetAccountRepo()
	err := ar.SendToBox(address.HashAddress(info.Hash), account.BoxInbox, msgID)
	if err != nil {
		// Something went wrong.. let's try and move the message back to the retry queue
		logrus.Warnf("cannot deliver %s locally. Moving to retry queue", msgID)
		MoveToRetryQueue(msgID)
	}

	return nil
}

// deliverRemote uploads a message to a remote mail server. For this to work it first needs to fetch a
// ticket from that server. Either that ticket is supplied, or we need to do proof-of-work first before
// we get the ticket. Once we have the ticket, we can upload the message to the server in the same way
// we upload a message from a client to a server.
func deliverRemote(header *message.Header, info *resolve.Info, msgID string) error {
	client, err := api.NewAnonymous(api.ClientOpts{
		Host:          info.Server,
		AllowInsecure: config.Server.Server.AllowInsecure,
	})
	if err != nil {
		return err
	}

	// Get upload ticket
	logrus.Tracef("getting ticket for %s:%s:%s", header.From.Addr, address.HashAddress(info.Hash), "")
	t, err := client.GetAnonymousTicket(header.From.Addr, address.HashAddress(info.Hash), "")
	if err != nil {
		return err
	}
	if !t.Valid {
		logrus.Debugf("ticket %s not valid. Need to do proof of work", t.ID)
		// Do proof of work. We have to wait for it. THis is ok as this is just a separate thread.
		t.Pow.Work(0)

		logrus.Debugf("work for %s is completed", t.ID)
		t, err = client.GetAnonymousTicketByProof(t.ID, t.Pow.Proof)
		if err != nil || !t.Valid {
			logrus.Warnf("Ticket for message %s not valid after proof of work, moving to retry queue", msgID)
			MoveToRetryQueue(msgID)
			return err
		}
	}

	// parallelize uploads
	g := new(errgroup.Group)
	g.Go(func() error {
		logrus.Tracef("uploading header for ticket %s", t.ID)
		return client.UploadHeader(*t, header)
	})
	g.Go(func() error {
		catalogPath, err := message.GetPath(message.SectionProcessing, msgID, "catalog")
		if err != nil {
			return err
		}

		catalogData, err := ioutil.ReadFile(catalogPath)
		if err != nil {
			return err
		}

		logrus.Tracef("uploading catalog for ticket %s", t.ID)
		return client.UploadCatalog(*t, catalogData)
	})

	messageFiles, err := message.GetFiles(message.SectionProcessing, msgID)
	if err != nil {
		_ = client.DeleteMessage(*t)
		return err
	}

	for _, messageFile := range messageFiles {
		// Store locally, otherwise the anonymous go function doesn't know which "block"
		mf := messageFile

		g.Go(func() error {
			// Open reader
			f, err := os.Open(mf.Path)
			if err != nil {
				return err
			}
			defer func() {
				_ = f.Close()
			}()

			logrus.Tracef("uploading block %s for ticket %s", mf.ID, t.ID)
			return client.UploadBlock(*t, mf.ID, f)
		})
	}

	// Wait until all are completed
	if err := g.Wait(); err != nil {
		logrus.Debugf("Error while uploading message %s: %s", msgID, err)
		_ = client.DeleteMessage(*t)
		return err
	}

	// All done, mark upload as completed
	logrus.Tracef("message completed for ticket %s", t.ID)
	err = client.CompleteUpload(*t)
	if err != nil {
		return err
	}

	// Remove local message from processing queue
	return message.RemoveMessage(message.SectionProcessing, msgID)
}
