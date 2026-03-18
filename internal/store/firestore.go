package store

import (
	"context"
	"fmt"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/mustafakarli/bdsmail/internal/model"
	"google.golang.org/api/iterator"
)

type DbFirestore struct {
	DbBase
	client *firestore.Client
}

// Uses shared docUser and docMessage structs from database.go

func NewDbFirestore(projectID string) (*DbFirestore, error) {
	ctx := context.Background()
	client, err := firestore.NewClient(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to create Firestore client: %w", err)
	}
	return &DbFirestore{client: client}, nil
}

func (db *DbFirestore) Close() error {
	return db.client.Close()
}

func (db *DbFirestore) GetQueries() map[string]string {
	return nil // Firestore does not use SQL queries
}

func (db *DbFirestore) users() *firestore.CollectionRef {
	return db.client.Collection("bdsmail-users")
}

func (db *DbFirestore) messages() *firestore.CollectionRef {
	return db.client.Collection("bdsmail-messages")
}

// User operations

func (db *DbFirestore) CreateUser(username, domain, displayName, passwordHash string) error {
	email := username + "@" + domain
	fu := docUser{
		Username:     username,
		Domain:       domain,
		DisplayName:  displayName,
		PasswordHash: passwordHash,
		CreatedAt:    time.Now().UTC().Format(time.RFC3339),
	}
	_, err := db.users().Doc(email).Create(context.Background(), fu)
	return err
}

func (db *DbFirestore) GetUser(username, domain string) (*model.User, error) {
	email := username + "@" + domain
	return db.getUserByKey(email)
}

func (db *DbFirestore) GetUserByEmail(email string) (*model.User, error) {
	return db.getUserByKey(email)
}

func (db *DbFirestore) getUserByKey(email string) (*model.User, error) {
	doc, err := db.users().Doc(email).Get(context.Background())
	if err != nil {
		return nil, fmt.Errorf("user not found: %s", email)
	}
	var fu docUser
	if err := doc.DataTo(&fu); err != nil {
		return nil, err
	}
	return db.docUserToModel(&fu), nil
}

func (db *DbFirestore) UserExistsByEmail(email string) bool {
	_, err := db.users().Doc(email).Get(context.Background())
	return err == nil
}

// Message operations

func (db *DbFirestore) SaveMessage(msg *model.Message) error {
	fm := docMessage{
		ID:          msg.ID,
		MessageID:   msg.MessageID,
		From:        msg.From,
		ToAddrs:     db.MarshalAddrs(msg.To),
		CCAddrs:     db.MarshalAddrs(msg.CC),
		BCCAddrs:    db.MarshalAddrs(msg.BCC),
		Subject:     msg.Subject,
		ContentType: msg.ContentType,
		Body:        msg.Body,
		Attachments: db.MarshalAttachments(msg.Attachments),
		GCSKey:      msg.GCSKey,
		OwnerUser:   msg.OwnerUser,
		Folder:      msg.Folder,
		Seen:        msg.Seen,
		Deleted:     false,
		ReceivedAt:  msg.ReceivedAt.UTC().Format(time.RFC3339),
	}
	_, err := db.messages().Doc(msg.ID).Set(context.Background(), fm)
	return err
}

func (db *DbFirestore) ListMessages(ownerEmail, folder string) ([]*model.Message, error) {
	iter := db.messages().
		Where("owner_user", "==", ownerEmail).
		Where("folder", "==", folder).
		Where("deleted", "==", false).
		OrderBy("received_at", firestore.Desc).
		Documents(context.Background())
	return db.collectMessages(iter)
}

func (db *DbFirestore) ListAllMessages(ownerEmail string) ([]*model.Message, error) {
	iter := db.messages().
		Where("owner_user", "==", ownerEmail).
		Where("deleted", "==", false).
		OrderBy("received_at", firestore.Desc).
		Documents(context.Background())
	return db.collectMessages(iter)
}

func (db *DbFirestore) GetMessage(id string) (*model.Message, error) {
	doc, err := db.messages().Doc(id).Get(context.Background())
	if err != nil {
		return nil, fmt.Errorf("message not found: %s", id)
	}
	var fm docMessage
	if err := doc.DataTo(&fm); err != nil {
		return nil, err
	}
	return db.docMessageToModel(&fm), nil
}

func (db *DbFirestore) MarkSeen(id string) error {
	_, err := db.messages().Doc(id).Update(context.Background(), []firestore.Update{
		{Path: "seen", Value: true},
	})
	return err
}

func (db *DbFirestore) MarkDeleted(id string) error {
	_, err := db.messages().Doc(id).Update(context.Background(), []firestore.Update{
		{Path: "deleted", Value: true},
	})
	return err
}

func (db *DbFirestore) DeleteMessage(id string) error {
	_, err := db.messages().Doc(id).Delete(context.Background())
	return err
}

func (db *DbFirestore) collectMessages(iter *firestore.DocumentIterator) ([]*model.Message, error) {
	defer iter.Stop()
	var messages []*model.Message
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}
		var fm docMessage
		if err := doc.DataTo(&fm); err != nil {
			return nil, err
		}
		messages = append(messages, db.docMessageToModel(&fm))
	}
	return messages, nil
}

