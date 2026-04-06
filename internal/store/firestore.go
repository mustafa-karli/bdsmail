package store

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/mustafakarli/bdsmail/internal/model"
	"google.golang.org/api/iterator"
)

type DbFirestore struct {
	DbBase
	client *firestore.Client
}

// Firestore-specific document structs (not shared with DynamoDB).

type docAlias struct {
	AliasEmail   string `firestore:"alias_email"`
	TargetEmails string `firestore:"target_emails"` // JSON array
	IsCatchAll   bool   `firestore:"is_catch_all"`
}

type docMailingList struct {
	ListAddress string `firestore:"list_address"`
	Name        string `firestore:"name"`
	Description string `firestore:"description"`
	OwnerEmail  string `firestore:"owner_email"`
	CreatedAt   string `firestore:"created_at"`
}

type docListMember struct {
	ListAddress string `firestore:"list_address"`
	MemberEmail string `firestore:"member_email"`
}

type docFilter struct {
	ID         string `firestore:"id"`
	UserEmail  string `firestore:"user_email"`
	Name       string `firestore:"name"`
	Priority   int    `firestore:"priority"`
	Conditions string `firestore:"conditions"` // JSON
	Actions    string `firestore:"actions"`    // JSON
	Enabled    bool   `firestore:"enabled"`
}

type docAutoReply struct {
	UserEmail string `firestore:"user_email"`
	Enabled   bool   `firestore:"enabled"`
	Subject   string `firestore:"subject"`
	Body      string `firestore:"body"`
	StartDate string `firestore:"start_date"`
	EndDate   string `firestore:"end_date"`
}

type docAutoReplyLog struct {
	UserEmail   string `firestore:"user_email"`
	SenderEmail string `firestore:"sender_email"`
	SentAt      string `firestore:"sent_at"`
}

type docContact struct {
	ID         string `firestore:"id"`
	OwnerEmail string `firestore:"owner_email"`
	VCardData  string `firestore:"vcard_data"`
	ETag       string `firestore:"etag"`
	CreatedAt  string `firestore:"created_at"`
	UpdatedAt  string `firestore:"updated_at"`
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

// Collection helpers

func (db *DbFirestore) users() *firestore.CollectionRef {
	return db.client.Collection("bdsmail-users")
}

func (db *DbFirestore) messages() *firestore.CollectionRef {
	return db.client.Collection("bdsmail-messages")
}

func (db *DbFirestore) aliases() *firestore.CollectionRef {
	return db.client.Collection("bdsmail-aliases")
}

func (db *DbFirestore) mailingLists() *firestore.CollectionRef {
	return db.client.Collection("bdsmail-mailing-lists")
}

func (db *DbFirestore) listMembers() *firestore.CollectionRef {
	return db.client.Collection("bdsmail-list-members")
}

func (db *DbFirestore) filters() *firestore.CollectionRef {
	return db.client.Collection("bdsmail-filters")
}

func (db *DbFirestore) autoReplies() *firestore.CollectionRef {
	return db.client.Collection("bdsmail-auto-replies")
}

func (db *DbFirestore) autoReplyLog() *firestore.CollectionRef {
	return db.client.Collection("bdsmail-auto-reply-log")
}

func (db *DbFirestore) contacts() *firestore.CollectionRef {
	return db.client.Collection("bdsmail-contacts")
}

func (db *DbFirestore) domains() *firestore.CollectionRef {
	return db.client.Collection("bdsmail-domains")
}

func (db *DbFirestore) oauthClients() *firestore.CollectionRef {
	return db.client.Collection("bdsmail-oauth-clients")
}

func (db *DbFirestore) oauthCodes() *firestore.CollectionRef {
	return db.client.Collection("bdsmail-oauth-codes")
}

func (db *DbFirestore) oauthTokens() *firestore.CollectionRef {
	return db.client.Collection("bdsmail-oauth-tokens")
}

// ---- User operations ----

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

func (db *DbFirestore) ListUsers() ([]*model.User, error) {
	iter := db.users().
		OrderBy("domain", firestore.Asc).
		Documents(context.Background())
	defer iter.Stop()
	var users []*model.User
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}
		var fu docUser
		if err := doc.DataTo(&fu); err != nil {
			return nil, err
		}
		users = append(users, db.docUserToModel(&fu))
	}
	return users, nil
}

func (db *DbFirestore) ListUsersByDomain(domain string) ([]*model.User, error) {
	iter := db.users().
		Where("domain", "==", domain).
		Documents(context.Background())
	defer iter.Stop()
	var users []*model.User
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}
		var fu docUser
		if err := doc.DataTo(&fu); err != nil {
			return nil, err
		}
		users = append(users, db.docUserToModel(&fu))
	}
	return users, nil
}

func (db *DbFirestore) UpdateUser(email, displayName, passwordHash string) error {
	updates := []firestore.Update{
		{Path: "display_name", Value: displayName},
	}
	if passwordHash != "" {
		updates = append(updates, firestore.Update{Path: "password_hash", Value: passwordHash})
	}
	_, err := db.users().Doc(email).Update(context.Background(), updates)
	return err
}

func (db *DbFirestore) DeleteUser(email string) error {
	_, err := db.users().Doc(email).Delete(context.Background())
	return err
}

func (db *DbFirestore) DeleteUserMessages(email string) error {
	ctx := context.Background()
	iter := db.messages().
		Where("owner_user", "==", email).
		Documents(ctx)
	defer iter.Stop()
	batch := db.client.Batch()
	count := 0
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return err
		}
		batch.Delete(doc.Ref)
		count++
		if count%400 == 0 {
			if _, err := batch.Commit(ctx); err != nil {
				return err
			}
			batch = db.client.Batch()
		}
	}
	if count%400 != 0 {
		if _, err := batch.Commit(ctx); err != nil {
			return err
		}
	}
	return nil
}

// ---- Message operations ----

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

func (db *DbFirestore) CountUnread(ownerEmail, folder string) int {
	msgs, err := db.ListMessages(ownerEmail, folder)
	if err != nil {
		return 0
	}
	count := 0
	for _, m := range msgs {
		if !m.Seen {
			count++
		}
	}
	return count
}

func (db *DbFirestore) SearchMessages(ownerEmail, query string) ([]*model.Message, error) {
	iter := db.messages().
		Where("owner_user", "==", ownerEmail).
		Where("deleted", "==", false).
		OrderBy("received_at", firestore.Desc).
		Documents(context.Background())
	defer iter.Stop()
	lowerQuery := strings.ToLower(query)
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
		if strings.Contains(strings.ToLower(fm.Subject), lowerQuery) ||
			strings.Contains(strings.ToLower(fm.From), lowerQuery) ||
			strings.Contains(strings.ToLower(fm.ToAddrs), lowerQuery) ||
			strings.Contains(strings.ToLower(fm.Body), lowerQuery) {
			messages = append(messages, db.docMessageToModel(&fm))
		}
	}
	return messages, nil
}

func (db *DbFirestore) ListUserFolders(ownerEmail string) ([]string, error) {
	iter := db.messages().
		Where("owner_user", "==", ownerEmail).
		Where("deleted", "==", false).
		Documents(context.Background())
	defer iter.Stop()
	seen := make(map[string]bool)
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
		seen[fm.Folder] = true
	}
	var folders []string
	for f := range seen {
		folders = append(folders, f)
	}
	return folders, nil
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

// ---- Alias operations ----

func (db *DbFirestore) CreateAlias(aliasEmail string, targetEmails []string) error {
	isCatchAll := len(aliasEmail) > 0 && aliasEmail[0] == '@'
	targetsJSON, _ := json.Marshal(targetEmails)
	da := docAlias{
		AliasEmail:   aliasEmail,
		TargetEmails: string(targetsJSON),
		IsCatchAll:   isCatchAll,
	}
	_, err := db.aliases().Doc(aliasEmail).Set(context.Background(), da)
	return err
}

func (db *DbFirestore) GetAlias(aliasEmail string) ([]string, error) {
	doc, err := db.aliases().Doc(aliasEmail).Get(context.Background())
	if err != nil {
		return nil, fmt.Errorf("alias not found: %s", aliasEmail)
	}
	var da docAlias
	if err := doc.DataTo(&da); err != nil {
		return nil, err
	}
	var targets []string
	json.Unmarshal([]byte(da.TargetEmails), &targets)
	return targets, nil
}

func (db *DbFirestore) ListAliases() ([]*model.Alias, error) {
	iter := db.aliases().Documents(context.Background())
	defer iter.Stop()
	var aliases []*model.Alias
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}
		var da docAlias
		if err := doc.DataTo(&da); err != nil {
			return nil, err
		}
		a := &model.Alias{
			AliasEmail: da.AliasEmail,
			IsCatchAll: da.IsCatchAll,
		}
		json.Unmarshal([]byte(da.TargetEmails), &a.TargetEmails)
		aliases = append(aliases, a)
	}
	return aliases, nil
}

func (db *DbFirestore) UpdateAlias(aliasEmail string, targetEmails []string) error {
	targetsJSON, _ := json.Marshal(targetEmails)
	_, err := db.aliases().Doc(aliasEmail).Update(context.Background(), []firestore.Update{
		{Path: "target_emails", Value: string(targetsJSON)},
	})
	return err
}

func (db *DbFirestore) DeleteAlias(aliasEmail string) error {
	_, err := db.aliases().Doc(aliasEmail).Delete(context.Background())
	return err
}

func (db *DbFirestore) GetCatchAll(domain string) ([]string, error) {
	return db.GetAlias("@" + domain)
}

// ---- Mailing list operations ----

func (db *DbFirestore) CreateMailingList(listAddr, name, description, ownerEmail string) error {
	dm := docMailingList{
		ListAddress: listAddr,
		Name:        name,
		Description: description,
		OwnerEmail:  ownerEmail,
		CreatedAt:   time.Now().UTC().Format(time.RFC3339),
	}
	_, err := db.mailingLists().Doc(listAddr).Set(context.Background(), dm)
	return err
}

func (db *DbFirestore) GetMailingList(listAddr string) (*model.MailingList, error) {
	doc, err := db.mailingLists().Doc(listAddr).Get(context.Background())
	if err != nil {
		return nil, fmt.Errorf("mailing list not found: %s", listAddr)
	}
	var dm docMailingList
	if err := doc.DataTo(&dm); err != nil {
		return nil, err
	}
	createdAt, _ := time.Parse(time.RFC3339, dm.CreatedAt)
	return &model.MailingList{
		ListAddress: dm.ListAddress,
		Name:        dm.Name,
		Description: dm.Description,
		OwnerEmail:  dm.OwnerEmail,
		CreatedAt:   createdAt,
	}, nil
}

func (db *DbFirestore) ListMailingLists() ([]*model.MailingList, error) {
	iter := db.mailingLists().Documents(context.Background())
	defer iter.Stop()
	var lists []*model.MailingList
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}
		var dm docMailingList
		if err := doc.DataTo(&dm); err != nil {
			return nil, err
		}
		createdAt, _ := time.Parse(time.RFC3339, dm.CreatedAt)
		lists = append(lists, &model.MailingList{
			ListAddress: dm.ListAddress,
			Name:        dm.Name,
			Description: dm.Description,
			OwnerEmail:  dm.OwnerEmail,
			CreatedAt:   createdAt,
		})
	}
	return lists, nil
}

func (db *DbFirestore) DeleteMailingList(listAddr string) error {
	// Delete all members first
	ctx := context.Background()
	iter := db.listMembers().
		Where("list_address", "==", listAddr).
		Documents(ctx)
	defer iter.Stop()
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return err
		}
		doc.Ref.Delete(ctx)
	}
	// Delete the list itself
	_, err := db.mailingLists().Doc(listAddr).Delete(ctx)
	return err
}

func (db *DbFirestore) AddListMember(listAddr, memberEmail string) error {
	docID := listAddr + "_" + memberEmail
	dm := docListMember{
		ListAddress: listAddr,
		MemberEmail: memberEmail,
	}
	_, err := db.listMembers().Doc(docID).Set(context.Background(), dm)
	return err
}

func (db *DbFirestore) RemoveListMember(listAddr, memberEmail string) error {
	docID := listAddr + "_" + memberEmail
	_, err := db.listMembers().Doc(docID).Delete(context.Background())
	return err
}

func (db *DbFirestore) GetListMembers(listAddr string) ([]string, error) {
	iter := db.listMembers().
		Where("list_address", "==", listAddr).
		Documents(context.Background())
	defer iter.Stop()
	var members []string
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}
		var dm docListMember
		if err := doc.DataTo(&dm); err != nil {
			return nil, err
		}
		members = append(members, dm.MemberEmail)
	}
	return members, nil
}

func (db *DbFirestore) IsMailingList(email string) bool {
	_, err := db.mailingLists().Doc(email).Get(context.Background())
	return err == nil
}

// ---- Filter operations ----

func (db *DbFirestore) CreateFilter(filter *model.Filter) error {
	condJSON, _ := json.Marshal(filter.Conditions)
	actJSON, _ := json.Marshal(filter.Actions)
	df := docFilter{
		ID:         filter.ID,
		UserEmail:  filter.UserEmail,
		Name:       filter.Name,
		Priority:   filter.Priority,
		Conditions: string(condJSON),
		Actions:    string(actJSON),
		Enabled:    filter.Enabled,
	}
	_, err := db.filters().Doc(filter.ID).Set(context.Background(), df)
	return err
}

func (db *DbFirestore) ListFilters(userEmail string) ([]*model.Filter, error) {
	iter := db.filters().
		Where("user_email", "==", userEmail).
		OrderBy("priority", firestore.Desc).
		Documents(context.Background())
	defer iter.Stop()
	var filters []*model.Filter
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}
		var df docFilter
		if err := doc.DataTo(&df); err != nil {
			return nil, err
		}
		f := &model.Filter{
			ID:        df.ID,
			UserEmail: df.UserEmail,
			Name:      df.Name,
			Priority:  df.Priority,
			Enabled:   df.Enabled,
		}
		json.Unmarshal([]byte(df.Conditions), &f.Conditions)
		json.Unmarshal([]byte(df.Actions), &f.Actions)
		filters = append(filters, f)
	}
	return filters, nil
}

func (db *DbFirestore) UpdateFilter(filter *model.Filter) error {
	condJSON, _ := json.Marshal(filter.Conditions)
	actJSON, _ := json.Marshal(filter.Actions)
	df := docFilter{
		ID:         filter.ID,
		UserEmail:  filter.UserEmail,
		Name:       filter.Name,
		Priority:   filter.Priority,
		Conditions: string(condJSON),
		Actions:    string(actJSON),
		Enabled:    filter.Enabled,
	}
	_, err := db.filters().Doc(filter.ID).Set(context.Background(), df)
	return err
}

func (db *DbFirestore) DeleteFilter(id string) error {
	_, err := db.filters().Doc(id).Delete(context.Background())
	return err
}

// ---- Auto-reply operations ----

func (db *DbFirestore) SetAutoReply(reply *model.AutoReply) error {
	da := docAutoReply{
		UserEmail: reply.UserEmail,
		Enabled:   reply.Enabled,
		Subject:   reply.Subject,
		Body:      reply.Body,
		StartDate: reply.StartDate.UTC().Format(time.RFC3339),
		EndDate:   reply.EndDate.UTC().Format(time.RFC3339),
	}
	_, err := db.autoReplies().Doc(reply.UserEmail).Set(context.Background(), da)
	return err
}

func (db *DbFirestore) GetAutoReply(userEmail string) (*model.AutoReply, error) {
	doc, err := db.autoReplies().Doc(userEmail).Get(context.Background())
	if err != nil {
		return nil, fmt.Errorf("auto-reply not found: %s", userEmail)
	}
	var da docAutoReply
	if err := doc.DataTo(&da); err != nil {
		return nil, err
	}
	startDate, _ := time.Parse(time.RFC3339, da.StartDate)
	endDate, _ := time.Parse(time.RFC3339, da.EndDate)
	return &model.AutoReply{
		UserEmail: da.UserEmail,
		Enabled:   da.Enabled,
		Subject:   da.Subject,
		Body:      da.Body,
		StartDate: startDate,
		EndDate:   endDate,
	}, nil
}

func (db *DbFirestore) DeleteAutoReply(userEmail string) error {
	_, err := db.autoReplies().Doc(userEmail).Delete(context.Background())
	return err
}

func (db *DbFirestore) RecordAutoReplySent(userEmail, senderEmail string) error {
	docID := userEmail + "_" + senderEmail
	dl := docAutoReplyLog{
		UserEmail:   userEmail,
		SenderEmail: senderEmail,
		SentAt:      time.Now().UTC().Format(time.RFC3339),
	}
	_, err := db.autoReplyLog().Doc(docID).Set(context.Background(), dl)
	return err
}

func (db *DbFirestore) HasAutoRepliedRecently(userEmail, senderEmail string, cooldown time.Duration) bool {
	docID := userEmail + "_" + senderEmail
	doc, err := db.autoReplyLog().Doc(docID).Get(context.Background())
	if err != nil {
		return false
	}
	var dl docAutoReplyLog
	if err := doc.DataTo(&dl); err != nil {
		return false
	}
	sentAt, err := time.Parse(time.RFC3339, dl.SentAt)
	if err != nil {
		return false
	}
	return time.Since(sentAt) < cooldown
}

// ---- Contact operations ----

func (db *DbFirestore) CreateContact(contact *model.Contact) error {
	now := time.Now().UTC().Format(time.RFC3339)
	dc := docContact{
		ID:         contact.ID,
		OwnerEmail: contact.OwnerEmail,
		VCardData:  contact.VCardData,
		ETag:       contact.ETag,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	_, err := db.contacts().Doc(contact.ID).Set(context.Background(), dc)
	return err
}

func (db *DbFirestore) GetContact(id string) (*model.Contact, error) {
	doc, err := db.contacts().Doc(id).Get(context.Background())
	if err != nil {
		return nil, fmt.Errorf("contact not found: %s", id)
	}
	var dc docContact
	if err := doc.DataTo(&dc); err != nil {
		return nil, err
	}
	createdAt, _ := time.Parse(time.RFC3339, dc.CreatedAt)
	updatedAt, _ := time.Parse(time.RFC3339, dc.UpdatedAt)
	return &model.Contact{
		ID:         dc.ID,
		OwnerEmail: dc.OwnerEmail,
		VCardData:  dc.VCardData,
		ETag:       dc.ETag,
		CreatedAt:  createdAt,
		UpdatedAt:  updatedAt,
	}, nil
}

func (db *DbFirestore) ListContacts(ownerEmail string) ([]*model.Contact, error) {
	iter := db.contacts().
		Where("owner_email", "==", ownerEmail).
		Documents(context.Background())
	defer iter.Stop()
	var contacts []*model.Contact
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}
		var dc docContact
		if err := doc.DataTo(&dc); err != nil {
			return nil, err
		}
		createdAt, _ := time.Parse(time.RFC3339, dc.CreatedAt)
		updatedAt, _ := time.Parse(time.RFC3339, dc.UpdatedAt)
		contacts = append(contacts, &model.Contact{
			ID:         dc.ID,
			OwnerEmail: dc.OwnerEmail,
			VCardData:  dc.VCardData,
			ETag:       dc.ETag,
			CreatedAt:  createdAt,
			UpdatedAt:  updatedAt,
		})
	}
	return contacts, nil
}

func (db *DbFirestore) UpdateContact(contact *model.Contact) error {
	_, err := db.contacts().Doc(contact.ID).Update(context.Background(), []firestore.Update{
		{Path: "vcard_data", Value: contact.VCardData},
		{Path: "etag", Value: contact.ETag},
		{Path: "updated_at", Value: time.Now().UTC().Format(time.RFC3339)},
	})
	return err
}

func (db *DbFirestore) DeleteContact(id string) error {
	_, err := db.contacts().Doc(id).Delete(context.Background())
	return err
}

// --- Auth / 2FA operations (stub — full implementation needed for Firestore) ---

func (db *DbFirestore) Enable2FA(email, secret, backupCodes string) error {
	return fmt.Errorf("2FA not yet implemented for Firestore")
}
func (db *DbFirestore) Disable2FA(email string) error {
	return fmt.Errorf("2FA not yet implemented for Firestore")
}
func (db *DbFirestore) Get2FAStatus(email string) (bool, string, string, error) {
	return false, "", "", nil
}
func (db *DbFirestore) CreateTrustedDevice(device *model.TrustedDevice) error {
	return fmt.Errorf("trusted devices not yet implemented for Firestore")
}
func (db *DbFirestore) IsTrustedDevice(email, fingerprint string) (bool, error) { return false, nil }
func (db *DbFirestore) ListTrustedDevices(email string) ([]*model.TrustedDevice, error) {
	return nil, nil
}
func (db *DbFirestore) RevokeTrustedDevice(id string) error { return nil }
func (db *DbFirestore) UpdateDeviceLastSeen(id string) error { return nil }
func (db *DbFirestore) CreateOTP(otp *model.OTP) error {
	return fmt.Errorf("OTP not yet implemented for Firestore")
}
func (db *DbFirestore) GetOTP(email string) (*model.OTP, error) {
	return nil, fmt.Errorf("not found")
}
func (db *DbFirestore) IncrementOTPAttempts(email string) error { return nil }
func (db *DbFirestore) ClearOTP(email string) error            { return nil }
func (db *DbFirestore) CreateLoginToken(token *model.LoginToken) error {
	return fmt.Errorf("login tokens not yet implemented for Firestore")
}
func (db *DbFirestore) GetLoginToken(token string) (*model.LoginToken, error) {
	return nil, fmt.Errorf("not found")
}
func (db *DbFirestore) DeleteLoginToken(token string) error { return nil }

// --- Domain operations ---

type docDomain struct {
	Name       string `firestore:"name"`
	APIKeyHash string `firestore:"api_key_hash"`
	SESStatus  string `firestore:"ses_status"`
	DKIMStatus string `firestore:"dkim_status"`
	Status     string `firestore:"status"`
	CreatedBy  string `firestore:"created_by"`
	CreatedAt  string `firestore:"created_at"`
}

func (db *DbFirestore) CreateDomain(domain *model.Domain) error {
	dd := docDomain{
		Name: domain.Name, APIKeyHash: domain.APIKeyHash,
		SESStatus: domain.SESStatus, DKIMStatus: domain.DKIMStatus,
		Status: domain.Status, CreatedBy: domain.CreatedBy,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}
	_, err := db.domains().Doc(domain.Name).Set(context.Background(), dd)
	return err
}

func (db *DbFirestore) GetDomain(name string) (*model.Domain, error) {
	doc, err := db.domains().Doc(name).Get(context.Background())
	if err != nil {
		return nil, fmt.Errorf("domain not found: %s", name)
	}
	var dd docDomain
	doc.DataTo(&dd)
	createdAt, _ := time.Parse(time.RFC3339, dd.CreatedAt)
	return &model.Domain{Name: dd.Name, APIKeyHash: dd.APIKeyHash, SESStatus: dd.SESStatus, DKIMStatus: dd.DKIMStatus, Status: dd.Status, CreatedBy: dd.CreatedBy, CreatedAt: createdAt}, nil
}

func (db *DbFirestore) ListDomains() ([]*model.Domain, error) {
	iter := db.domains().Documents(context.Background())
	defer iter.Stop()
	var domains []*model.Domain
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}
		var dd docDomain
		doc.DataTo(&dd)
		createdAt, _ := time.Parse(time.RFC3339, dd.CreatedAt)
		domains = append(domains, &model.Domain{Name: dd.Name, APIKeyHash: dd.APIKeyHash, SESStatus: dd.SESStatus, DKIMStatus: dd.DKIMStatus, Status: dd.Status, CreatedBy: dd.CreatedBy, CreatedAt: createdAt})
	}
	return domains, nil
}

func (db *DbFirestore) UpdateDomainStatus(name, sesStatus, dkimStatus string) error {
	_, err := db.domains().Doc(name).Update(context.Background(), []firestore.Update{
		{Path: "ses_status", Value: sesStatus},
		{Path: "dkim_status", Value: dkimStatus},
	})
	return err
}

func (db *DbFirestore) DeleteDomain(name string) error {
	_, err := db.domains().Doc(name).Delete(context.Background())
	return err
}

// --- OAuth operations ---

type docOAuthClient struct {
	ID          string `firestore:"id"`
	Name        string `firestore:"name"`
	ClientID    string `firestore:"client_id"`
	SecretHash  string `firestore:"secret_hash"`
	RedirectURI string `firestore:"redirect_uri"`
	Domain      string `firestore:"domain"`
	CreatedBy   string `firestore:"created_by"`
	CreatedAt   string `firestore:"created_at"`
}

type docOAuthCode struct {
	Code        string `firestore:"code"`
	ClientID    string `firestore:"client_id"`
	UserEmail   string `firestore:"user_email"`
	RedirectURI string `firestore:"redirect_uri"`
	Scope       string `firestore:"scope"`
	Nonce       string `firestore:"nonce"`
	ExpiresAt   string `firestore:"expires_at"`
	Used        bool   `firestore:"used"`
}

type docOAuthToken struct {
	Token     string `firestore:"token"`
	ClientID  string `firestore:"client_id"`
	UserEmail string `firestore:"user_email"`
	Scope     string `firestore:"scope"`
	ExpiresAt string `firestore:"expires_at"`
}

func (db *DbFirestore) CreateOAuthClient(client *model.OAuthClient) error {
	dc := docOAuthClient{
		ID: client.ID, Name: client.Name, ClientID: client.ClientID,
		SecretHash: client.SecretHash, RedirectURI: client.RedirectURI,
		Domain: client.Domain, CreatedBy: client.CreatedBy,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}
	_, err := db.oauthClients().Doc(client.ID).Set(context.Background(), dc)
	return err
}

func (db *DbFirestore) GetOAuthClient(clientID string) (*model.OAuthClient, error) {
	iter := db.oauthClients().Where("client_id", "==", clientID).Limit(1).Documents(context.Background())
	defer iter.Stop()
	doc, err := iter.Next()
	if err != nil {
		return nil, fmt.Errorf("oauth client not found: %s", clientID)
	}
	var dc docOAuthClient
	doc.DataTo(&dc)
	createdAt, _ := time.Parse(time.RFC3339, dc.CreatedAt)
	return &model.OAuthClient{ID: dc.ID, Name: dc.Name, ClientID: dc.ClientID, SecretHash: dc.SecretHash, RedirectURI: dc.RedirectURI, Domain: dc.Domain, CreatedBy: dc.CreatedBy, CreatedAt: createdAt}, nil
}

func (db *DbFirestore) ListOAuthClients(domain string) ([]*model.OAuthClient, error) {
	iter := db.oauthClients().Where("domain", "==", domain).Documents(context.Background())
	defer iter.Stop()
	var clients []*model.OAuthClient
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}
		var dc docOAuthClient
		doc.DataTo(&dc)
		createdAt, _ := time.Parse(time.RFC3339, dc.CreatedAt)
		clients = append(clients, &model.OAuthClient{ID: dc.ID, Name: dc.Name, ClientID: dc.ClientID, SecretHash: dc.SecretHash, RedirectURI: dc.RedirectURI, Domain: dc.Domain, CreatedBy: dc.CreatedBy, CreatedAt: createdAt})
	}
	return clients, nil
}

func (db *DbFirestore) DeleteOAuthClient(id string) error {
	_, err := db.oauthClients().Doc(id).Delete(context.Background())
	return err
}

func (db *DbFirestore) CreateOAuthCode(code *model.OAuthCode) error {
	dc := docOAuthCode{
		Code: code.Code, ClientID: code.ClientID, UserEmail: code.UserEmail,
		RedirectURI: code.RedirectURI, Scope: code.Scope, Nonce: code.Nonce,
		ExpiresAt: code.ExpiresAt.UTC().Format(time.RFC3339), Used: false,
	}
	_, err := db.oauthCodes().Doc(code.Code).Set(context.Background(), dc)
	return err
}

func (db *DbFirestore) GetOAuthCode(code string) (*model.OAuthCode, error) {
	doc, err := db.oauthCodes().Doc(code).Get(context.Background())
	if err != nil {
		return nil, fmt.Errorf("oauth code not found")
	}
	var dc docOAuthCode
	doc.DataTo(&dc)
	expiresAt, _ := time.Parse(time.RFC3339, dc.ExpiresAt)
	return &model.OAuthCode{Code: dc.Code, ClientID: dc.ClientID, UserEmail: dc.UserEmail, RedirectURI: dc.RedirectURI, Scope: dc.Scope, Nonce: dc.Nonce, ExpiresAt: expiresAt, Used: dc.Used}, nil
}

func (db *DbFirestore) MarkOAuthCodeUsed(code string) error {
	_, err := db.oauthCodes().Doc(code).Update(context.Background(), []firestore.Update{
		{Path: "used", Value: true},
	})
	return err
}

func (db *DbFirestore) CreateOAuthToken(token *model.OAuthToken) error {
	dt := docOAuthToken{
		Token: token.Token, ClientID: token.ClientID, UserEmail: token.UserEmail,
		Scope: token.Scope, ExpiresAt: token.ExpiresAt.UTC().Format(time.RFC3339),
	}
	_, err := db.oauthTokens().Doc(token.Token).Set(context.Background(), dt)
	return err
}

func (db *DbFirestore) GetOAuthToken(token string) (*model.OAuthToken, error) {
	doc, err := db.oauthTokens().Doc(token).Get(context.Background())
	if err != nil {
		return nil, fmt.Errorf("oauth token not found")
	}
	var dt docOAuthToken
	doc.DataTo(&dt)
	expiresAt, _ := time.Parse(time.RFC3339, dt.ExpiresAt)
	return &model.OAuthToken{Token: dt.Token, ClientID: dt.ClientID, UserEmail: dt.UserEmail, Scope: dt.Scope, ExpiresAt: expiresAt}, nil
}

func (db *DbFirestore) DeleteOAuthToken(token string) error {
	_, err := db.oauthTokens().Doc(token).Delete(context.Background())
	return err
}
