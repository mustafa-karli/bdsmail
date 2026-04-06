package store

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/mustafakarli/bdsmail/internal/model"
)

const (
	usersTableName        = "bdsmail-users"
	messagesTableName     = "bdsmail-messages"
	folderIndexName       = "folder-index"
	aliasesTableName      = "bdsmail-aliases"
	mailingListsTableName = "bdsmail-mailing-lists"
	listMembersTableName  = "bdsmail-list-members"
	filtersTableName      = "bdsmail-filters"
	autoRepliesTableName  = "bdsmail-auto-replies"
	autoReplyLogTableName = "bdsmail-auto-reply-log"
	contactsTableName     = "bdsmail-contacts"
	domainsTableName      = "bdsmail-domains"
	oauthClientsTableName = "bdsmail-oauth-clients"
	oauthCodesTableName   = "bdsmail-oauth-codes"
	oauthTokensTableName  = "bdsmail-oauth-tokens"
)

type DbDynamo struct {
	DbBase
	client *dynamodb.Client
}

// Uses shared docUser and docMessage structs from database.go

func NewDbDynamo(region string) (*DbDynamo, error) {
	cfg, err := awsconfig.LoadDefaultConfig(context.Background(), awsconfig.WithRegion(region))
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}
	client := dynamodb.NewFromConfig(cfg)
	db := &DbDynamo{client: client}
	return db, nil
}

func (db *DbDynamo) Close() error {
	return nil
}

func (db *DbDynamo) GetQueries() map[string]string {
	return nil // DynamoDB does not use SQL queries
}

// Table creation is handled by sql/init_dynamodb.go during deployment.

// User operations

func (db *DbDynamo) CreateUser(username, domain, displayName, passwordHash string) error {
	u := docUser{
		Email:        username + "@" + domain,
		Username:     username,
		Domain:       domain,
		DisplayName:  displayName,
		PasswordHash: passwordHash,
		CreatedAt:    time.Now().UTC().Format(time.RFC3339),
	}
	item, err := attributevalue.MarshalMap(u)
	if err != nil {
		return err
	}
	_, err = db.client.PutItem(context.Background(), &dynamodb.PutItemInput{
		TableName:           aws.String(usersTableName),
		Item:                item,
		ConditionExpression: aws.String("attribute_not_exists(email)"),
	})
	return err
}

func (db *DbDynamo) GetUser(username, domain string) (*model.User, error) {
	email := username + "@" + domain
	return db.getUserByKey(email)
}

func (db *DbDynamo) GetUserByEmail(email string) (*model.User, error) {
	return db.getUserByKey(email)
}

func (db *DbDynamo) getUserByKey(email string) (*model.User, error) {
	result, err := db.client.GetItem(context.Background(), &dynamodb.GetItemInput{
		TableName: aws.String(usersTableName),
		Key: map[string]types.AttributeValue{
			"email": &types.AttributeValueMemberS{Value: email},
		},
	})
	if err != nil {
		return nil, err
	}
	if result.Item == nil {
		return nil, fmt.Errorf("user not found: %s", email)
	}
	var du docUser
	if err := attributevalue.UnmarshalMap(result.Item, &du); err != nil {
		return nil, err
	}
	return db.docUserToModel(&du), nil
}

func (db *DbDynamo) UserExistsByEmail(email string) bool {
	result, err := db.client.GetItem(context.Background(), &dynamodb.GetItemInput{
		TableName: aws.String(usersTableName),
		Key: map[string]types.AttributeValue{
			"email": &types.AttributeValueMemberS{Value: email},
		},
		ProjectionExpression: aws.String("email"),
	})
	return err == nil && result.Item != nil
}

// Message operations

func (db *DbDynamo) SaveMessage(msg *model.Message) error {
	dm := docMessage{
		ID:          msg.ID,
		OwnerUser:   msg.OwnerUser,
		SortKey:     msg.ReceivedAt.UTC().Format(time.RFC3339Nano) + "#" + msg.ID,
		MessageID:   msg.MessageID,
		From:        msg.From,
		ToAddrs:     db.MarshalAddrs(msg.To),
		CCAddrs:     db.MarshalAddrs(msg.CC),
		BCCAddrs:    db.MarshalAddrs(msg.BCC),
		Subject:     msg.Subject,
		ContentType: msg.ContentType,
		Body:        msg.Body,
		GCSKey:      msg.GCSKey,
		Folder:      msg.Folder,
		Seen:        msg.Seen,
		Deleted:     false,
		ReceivedAt:  msg.ReceivedAt.UTC().Format(time.RFC3339),
	}
	item, err := attributevalue.MarshalMap(dm)
	if err != nil {
		return err
	}
	_, err = db.client.PutItem(context.Background(), &dynamodb.PutItemInput{
		TableName: aws.String(messagesTableName),
		Item:      item,
	})
	return err
}

func (db *DbDynamo) ListMessages(ownerEmail, folder string) ([]*model.Message, error) {
	result, err := db.client.Query(context.Background(), &dynamodb.QueryInput{
		TableName:              aws.String(messagesTableName),
		KeyConditionExpression: aws.String("owner_user = :owner"),
		FilterExpression:       aws.String("folder = :folder AND deleted = :false"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":owner":  &types.AttributeValueMemberS{Value: ownerEmail},
			":folder": &types.AttributeValueMemberS{Value: folder},
			":false":  &types.AttributeValueMemberBOOL{Value: false},
		},
		ScanIndexForward: aws.Bool(false), // DESC order
	})
	if err != nil {
		return nil, err
	}
	return db.unmarshalMessages(result.Items)
}

func (db *DbDynamo) ListAllMessages(ownerEmail string) ([]*model.Message, error) {
	result, err := db.client.Query(context.Background(), &dynamodb.QueryInput{
		TableName:              aws.String(messagesTableName),
		KeyConditionExpression: aws.String("owner_user = :owner"),
		FilterExpression:       aws.String("deleted = :false"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":owner": &types.AttributeValueMemberS{Value: ownerEmail},
			":false": &types.AttributeValueMemberBOOL{Value: false},
		},
		ScanIndexForward: aws.Bool(false),
	})
	if err != nil {
		return nil, err
	}
	return db.unmarshalMessages(result.Items)
}

func (db *DbDynamo) GetMessage(id string) (*model.Message, error) {
	// Use the id-index GSI to find by message ID
	result, err := db.client.Query(context.Background(), &dynamodb.QueryInput{
		TableName:              aws.String(messagesTableName),
		IndexName:              aws.String("id-index"),
		KeyConditionExpression: aws.String("id = :id"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":id": &types.AttributeValueMemberS{Value: id},
		},
		Limit: aws.Int32(1),
	})
	if err != nil {
		return nil, err
	}
	if len(result.Items) == 0 {
		return nil, fmt.Errorf("message not found: %s", id)
	}
	msgs, err := db.unmarshalMessages(result.Items)
	if err != nil {
		return nil, err
	}
	return msgs[0], nil
}

func (db *DbDynamo) MarkSeen(id string) error {
	return db.updateMessageField(id, "seen", &types.AttributeValueMemberBOOL{Value: true})
}

func (db *DbDynamo) MarkDeleted(id string) error {
	return db.updateMessageField(id, "deleted", &types.AttributeValueMemberBOOL{Value: true})
}

func (db *DbDynamo) DeleteMessage(id string) error {
	// First get the message to find its composite key
	msg, err := db.GetMessage(id)
	if err != nil {
		return err
	}
	sortKey := msg.ReceivedAt.UTC().Format(time.RFC3339Nano) + "#" + msg.ID
	_, err = db.client.DeleteItem(context.Background(), &dynamodb.DeleteItemInput{
		TableName: aws.String(messagesTableName),
		Key: map[string]types.AttributeValue{
			"owner_user": &types.AttributeValueMemberS{Value: msg.OwnerUser},
			"sort_key":   &types.AttributeValueMemberS{Value: sortKey},
		},
	})
	return err
}

func (db *DbDynamo) updateMessageField(id string, field string, value types.AttributeValue) error {
	msg, err := db.GetMessage(id)
	if err != nil {
		return err
	}
	sortKey := msg.ReceivedAt.UTC().Format(time.RFC3339Nano) + "#" + msg.ID
	_, err = db.client.UpdateItem(context.Background(), &dynamodb.UpdateItemInput{
		TableName: aws.String(messagesTableName),
		Key: map[string]types.AttributeValue{
			"owner_user": &types.AttributeValueMemberS{Value: msg.OwnerUser},
			"sort_key":   &types.AttributeValueMemberS{Value: sortKey},
		},
		UpdateExpression: aws.String("SET #f = :v"),
		ExpressionAttributeNames: map[string]string{
			"#f": field,
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":v": value,
		},
	})
	return err
}

func (db *DbDynamo) unmarshalMessages(items []map[string]types.AttributeValue) ([]*model.Message, error) {
	var messages []*model.Message
	for _, item := range items {
		var dm docMessage
		if err := attributevalue.UnmarshalMap(item, &dm); err != nil {
			return nil, err
		}
		messages = append(messages, db.docMessageToModel(&dm))
	}
	return messages, nil
}

// --- User provisioning ---

func (db *DbDynamo) ListUsers() ([]*model.User, error) {
	result, err := db.client.Scan(context.Background(), &dynamodb.ScanInput{
		TableName: aws.String(usersTableName),
	})
	if err != nil {
		return nil, err
	}
	var users []*model.User
	for _, item := range result.Items {
		var du docUser
		if err := attributevalue.UnmarshalMap(item, &du); err != nil {
			return nil, err
		}
		users = append(users, db.docUserToModel(&du))
	}
	return users, nil
}

func (db *DbDynamo) ListUsersByDomain(domain string) ([]*model.User, error) {
	result, err := db.client.Scan(context.Background(), &dynamodb.ScanInput{
		TableName:        aws.String(usersTableName),
		FilterExpression: aws.String("#d = :domain"),
		ExpressionAttributeNames: map[string]string{
			"#d": "domain",
		},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":domain": &types.AttributeValueMemberS{Value: domain},
		},
	})
	if err != nil {
		return nil, err
	}
	var users []*model.User
	for _, item := range result.Items {
		var du docUser
		if err := attributevalue.UnmarshalMap(item, &du); err != nil {
			return nil, err
		}
		users = append(users, db.docUserToModel(&du))
	}
	return users, nil
}

func (db *DbDynamo) UpdateUser(email, displayName, passwordHash string) error {
	if passwordHash == "" {
		_, err := db.client.UpdateItem(context.Background(), &dynamodb.UpdateItemInput{
			TableName: aws.String(usersTableName),
			Key: map[string]types.AttributeValue{
				"email": &types.AttributeValueMemberS{Value: email},
			},
			UpdateExpression: aws.String("SET display_name = :dn"),
			ExpressionAttributeValues: map[string]types.AttributeValue{
				":dn": &types.AttributeValueMemberS{Value: displayName},
			},
		})
		return err
	}
	_, err := db.client.UpdateItem(context.Background(), &dynamodb.UpdateItemInput{
		TableName: aws.String(usersTableName),
		Key: map[string]types.AttributeValue{
			"email": &types.AttributeValueMemberS{Value: email},
		},
		UpdateExpression: aws.String("SET display_name = :dn, password_hash = :ph"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":dn": &types.AttributeValueMemberS{Value: displayName},
			":ph": &types.AttributeValueMemberS{Value: passwordHash},
		},
	})
	return err
}

func (db *DbDynamo) DeleteUser(email string) error {
	_, err := db.client.DeleteItem(context.Background(), &dynamodb.DeleteItemInput{
		TableName: aws.String(usersTableName),
		Key: map[string]types.AttributeValue{
			"email": &types.AttributeValueMemberS{Value: email},
		},
	})
	return err
}

func (db *DbDynamo) DeleteUserMessages(email string) error {
	// Query all messages for this user
	result, err := db.client.Query(context.Background(), &dynamodb.QueryInput{
		TableName:              aws.String(messagesTableName),
		KeyConditionExpression: aws.String("owner_user = :owner"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":owner": &types.AttributeValueMemberS{Value: email},
		},
		ProjectionExpression: aws.String("owner_user, sort_key"),
	})
	if err != nil {
		return err
	}
	// Batch delete in groups of 25
	for i := 0; i < len(result.Items); i += 25 {
		end := i + 25
		if end > len(result.Items) {
			end = len(result.Items)
		}
		var writeRequests []types.WriteRequest
		for _, item := range result.Items[i:end] {
			writeRequests = append(writeRequests, types.WriteRequest{
				DeleteRequest: &types.DeleteRequest{
					Key: map[string]types.AttributeValue{
						"owner_user": item["owner_user"],
						"sort_key":   item["sort_key"],
					},
				},
			})
		}
		_, err := db.client.BatchWriteItem(context.Background(), &dynamodb.BatchWriteItemInput{
			RequestItems: map[string][]types.WriteRequest{
				messagesTableName: writeRequests,
			},
		})
		if err != nil {
			return err
		}
	}
	return nil
}

// --- Alias operations ---

func (db *DbDynamo) CreateAlias(aliasEmail string, targetEmails []string) error {
	isCatchAll := len(aliasEmail) > 0 && aliasEmail[0] == '@'
	targetsJSON, _ := json.Marshal(targetEmails)
	item := map[string]types.AttributeValue{
		"alias_email":   &types.AttributeValueMemberS{Value: aliasEmail},
		"target_emails": &types.AttributeValueMemberS{Value: string(targetsJSON)},
		"is_catch_all":  &types.AttributeValueMemberBOOL{Value: isCatchAll},
	}
	_, err := db.client.PutItem(context.Background(), &dynamodb.PutItemInput{
		TableName: aws.String(aliasesTableName),
		Item:      item,
	})
	return err
}

func (db *DbDynamo) GetAlias(aliasEmail string) ([]string, error) {
	result, err := db.client.GetItem(context.Background(), &dynamodb.GetItemInput{
		TableName: aws.String(aliasesTableName),
		Key: map[string]types.AttributeValue{
			"alias_email": &types.AttributeValueMemberS{Value: aliasEmail},
		},
	})
	if err != nil {
		return nil, err
	}
	if result.Item == nil {
		return nil, fmt.Errorf("alias not found: %s", aliasEmail)
	}
	targetsAttr, ok := result.Item["target_emails"]
	if !ok {
		return nil, fmt.Errorf("alias has no target_emails: %s", aliasEmail)
	}
	var targets []string
	json.Unmarshal([]byte(targetsAttr.(*types.AttributeValueMemberS).Value), &targets)
	return targets, nil
}

func (db *DbDynamo) ListAliases() ([]*model.Alias, error) {
	result, err := db.client.Scan(context.Background(), &dynamodb.ScanInput{
		TableName: aws.String(aliasesTableName),
	})
	if err != nil {
		return nil, err
	}
	var aliases []*model.Alias
	for _, item := range result.Items {
		a := &model.Alias{}
		if v, ok := item["alias_email"]; ok {
			a.AliasEmail = v.(*types.AttributeValueMemberS).Value
		}
		if v, ok := item["target_emails"]; ok {
			json.Unmarshal([]byte(v.(*types.AttributeValueMemberS).Value), &a.TargetEmails)
		}
		if v, ok := item["is_catch_all"]; ok {
			a.IsCatchAll = v.(*types.AttributeValueMemberBOOL).Value
		}
		aliases = append(aliases, a)
	}
	return aliases, nil
}

func (db *DbDynamo) UpdateAlias(aliasEmail string, targetEmails []string) error {
	targetsJSON, _ := json.Marshal(targetEmails)
	_, err := db.client.UpdateItem(context.Background(), &dynamodb.UpdateItemInput{
		TableName: aws.String(aliasesTableName),
		Key: map[string]types.AttributeValue{
			"alias_email": &types.AttributeValueMemberS{Value: aliasEmail},
		},
		UpdateExpression: aws.String("SET target_emails = :te"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":te": &types.AttributeValueMemberS{Value: string(targetsJSON)},
		},
	})
	return err
}

func (db *DbDynamo) DeleteAlias(aliasEmail string) error {
	_, err := db.client.DeleteItem(context.Background(), &dynamodb.DeleteItemInput{
		TableName: aws.String(aliasesTableName),
		Key: map[string]types.AttributeValue{
			"alias_email": &types.AttributeValueMemberS{Value: aliasEmail},
		},
	})
	return err
}

func (db *DbDynamo) GetCatchAll(domain string) ([]string, error) {
	return db.GetAlias("@" + domain)
}

// --- Mailing list operations ---

func (db *DbDynamo) CreateMailingList(listAddr, name, description, ownerEmail string) error {
	item := map[string]types.AttributeValue{
		"list_address": &types.AttributeValueMemberS{Value: listAddr},
		"name":         &types.AttributeValueMemberS{Value: name},
		"description":  &types.AttributeValueMemberS{Value: description},
		"owner_email":  &types.AttributeValueMemberS{Value: ownerEmail},
		"created_at":   &types.AttributeValueMemberS{Value: time.Now().UTC().Format(time.RFC3339)},
	}
	_, err := db.client.PutItem(context.Background(), &dynamodb.PutItemInput{
		TableName: aws.String(mailingListsTableName),
		Item:      item,
	})
	return err
}

func (db *DbDynamo) GetMailingList(listAddr string) (*model.MailingList, error) {
	result, err := db.client.GetItem(context.Background(), &dynamodb.GetItemInput{
		TableName: aws.String(mailingListsTableName),
		Key: map[string]types.AttributeValue{
			"list_address": &types.AttributeValueMemberS{Value: listAddr},
		},
	})
	if err != nil {
		return nil, err
	}
	if result.Item == nil {
		return nil, fmt.Errorf("mailing list not found: %s", listAddr)
	}
	ml := &model.MailingList{}
	if v, ok := result.Item["list_address"]; ok {
		ml.ListAddress = v.(*types.AttributeValueMemberS).Value
	}
	if v, ok := result.Item["name"]; ok {
		ml.Name = v.(*types.AttributeValueMemberS).Value
	}
	if v, ok := result.Item["description"]; ok {
		ml.Description = v.(*types.AttributeValueMemberS).Value
	}
	if v, ok := result.Item["owner_email"]; ok {
		ml.OwnerEmail = v.(*types.AttributeValueMemberS).Value
	}
	if v, ok := result.Item["created_at"]; ok {
		ml.CreatedAt, _ = time.Parse(time.RFC3339, v.(*types.AttributeValueMemberS).Value)
	}
	return ml, nil
}

func (db *DbDynamo) ListMailingLists() ([]*model.MailingList, error) {
	result, err := db.client.Scan(context.Background(), &dynamodb.ScanInput{
		TableName: aws.String(mailingListsTableName),
	})
	if err != nil {
		return nil, err
	}
	var lists []*model.MailingList
	for _, item := range result.Items {
		ml := &model.MailingList{}
		if v, ok := item["list_address"]; ok {
			ml.ListAddress = v.(*types.AttributeValueMemberS).Value
		}
		if v, ok := item["name"]; ok {
			ml.Name = v.(*types.AttributeValueMemberS).Value
		}
		if v, ok := item["description"]; ok {
			ml.Description = v.(*types.AttributeValueMemberS).Value
		}
		if v, ok := item["owner_email"]; ok {
			ml.OwnerEmail = v.(*types.AttributeValueMemberS).Value
		}
		if v, ok := item["created_at"]; ok {
			ml.CreatedAt, _ = time.Parse(time.RFC3339, v.(*types.AttributeValueMemberS).Value)
		}
		lists = append(lists, ml)
	}
	return lists, nil
}

func (db *DbDynamo) DeleteMailingList(listAddr string) error {
	// First delete all members
	members, err := db.client.Query(context.Background(), &dynamodb.QueryInput{
		TableName:              aws.String(listMembersTableName),
		KeyConditionExpression: aws.String("list_address = :la"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":la": &types.AttributeValueMemberS{Value: listAddr},
		},
		ProjectionExpression: aws.String("list_address, member_email"),
	})
	if err != nil {
		return err
	}
	for i := 0; i < len(members.Items); i += 25 {
		end := i + 25
		if end > len(members.Items) {
			end = len(members.Items)
		}
		var writeRequests []types.WriteRequest
		for _, item := range members.Items[i:end] {
			writeRequests = append(writeRequests, types.WriteRequest{
				DeleteRequest: &types.DeleteRequest{
					Key: map[string]types.AttributeValue{
						"list_address": item["list_address"],
						"member_email": item["member_email"],
					},
				},
			})
		}
		_, err := db.client.BatchWriteItem(context.Background(), &dynamodb.BatchWriteItemInput{
			RequestItems: map[string][]types.WriteRequest{
				listMembersTableName: writeRequests,
			},
		})
		if err != nil {
			return err
		}
	}
	// Then delete the list itself
	_, err = db.client.DeleteItem(context.Background(), &dynamodb.DeleteItemInput{
		TableName: aws.String(mailingListsTableName),
		Key: map[string]types.AttributeValue{
			"list_address": &types.AttributeValueMemberS{Value: listAddr},
		},
	})
	return err
}

func (db *DbDynamo) AddListMember(listAddr, memberEmail string) error {
	item := map[string]types.AttributeValue{
		"list_address": &types.AttributeValueMemberS{Value: listAddr},
		"member_email": &types.AttributeValueMemberS{Value: memberEmail},
	}
	_, err := db.client.PutItem(context.Background(), &dynamodb.PutItemInput{
		TableName: aws.String(listMembersTableName),
		Item:      item,
	})
	return err
}

func (db *DbDynamo) RemoveListMember(listAddr, memberEmail string) error {
	_, err := db.client.DeleteItem(context.Background(), &dynamodb.DeleteItemInput{
		TableName: aws.String(listMembersTableName),
		Key: map[string]types.AttributeValue{
			"list_address": &types.AttributeValueMemberS{Value: listAddr},
			"member_email": &types.AttributeValueMemberS{Value: memberEmail},
		},
	})
	return err
}

func (db *DbDynamo) GetListMembers(listAddr string) ([]string, error) {
	result, err := db.client.Query(context.Background(), &dynamodb.QueryInput{
		TableName:              aws.String(listMembersTableName),
		KeyConditionExpression: aws.String("list_address = :la"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":la": &types.AttributeValueMemberS{Value: listAddr},
		},
	})
	if err != nil {
		return nil, err
	}
	var members []string
	for _, item := range result.Items {
		if v, ok := item["member_email"]; ok {
			members = append(members, v.(*types.AttributeValueMemberS).Value)
		}
	}
	return members, nil
}

func (db *DbDynamo) IsMailingList(email string) bool {
	result, err := db.client.GetItem(context.Background(), &dynamodb.GetItemInput{
		TableName: aws.String(mailingListsTableName),
		Key: map[string]types.AttributeValue{
			"list_address": &types.AttributeValueMemberS{Value: email},
		},
		ProjectionExpression: aws.String("list_address"),
	})
	return err == nil && result.Item != nil
}

// --- Filter operations ---

func (db *DbDynamo) CreateFilter(filter *model.Filter) error {
	condJSON, _ := json.Marshal(filter.Conditions)
	actJSON, _ := json.Marshal(filter.Actions)
	item := map[string]types.AttributeValue{
		"user_email": &types.AttributeValueMemberS{Value: filter.UserEmail},
		"id":         &types.AttributeValueMemberS{Value: filter.ID},
		"name":       &types.AttributeValueMemberS{Value: filter.Name},
		"priority":   &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", filter.Priority)},
		"conditions": &types.AttributeValueMemberS{Value: string(condJSON)},
		"actions":    &types.AttributeValueMemberS{Value: string(actJSON)},
		"enabled":    &types.AttributeValueMemberBOOL{Value: filter.Enabled},
	}
	_, err := db.client.PutItem(context.Background(), &dynamodb.PutItemInput{
		TableName: aws.String(filtersTableName),
		Item:      item,
	})
	return err
}

func (db *DbDynamo) ListFilters(userEmail string) ([]*model.Filter, error) {
	result, err := db.client.Query(context.Background(), &dynamodb.QueryInput{
		TableName:              aws.String(filtersTableName),
		KeyConditionExpression: aws.String("user_email = :ue"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":ue": &types.AttributeValueMemberS{Value: userEmail},
		},
	})
	if err != nil {
		return nil, err
	}
	var filters []*model.Filter
	for _, item := range result.Items {
		f := &model.Filter{}
		if v, ok := item["id"]; ok {
			f.ID = v.(*types.AttributeValueMemberS).Value
		}
		if v, ok := item["user_email"]; ok {
			f.UserEmail = v.(*types.AttributeValueMemberS).Value
		}
		if v, ok := item["name"]; ok {
			f.Name = v.(*types.AttributeValueMemberS).Value
		}
		if v, ok := item["priority"]; ok {
			fmt.Sscanf(v.(*types.AttributeValueMemberN).Value, "%d", &f.Priority)
		}
		if v, ok := item["conditions"]; ok {
			json.Unmarshal([]byte(v.(*types.AttributeValueMemberS).Value), &f.Conditions)
		}
		if v, ok := item["actions"]; ok {
			json.Unmarshal([]byte(v.(*types.AttributeValueMemberS).Value), &f.Actions)
		}
		if v, ok := item["enabled"]; ok {
			f.Enabled = v.(*types.AttributeValueMemberBOOL).Value
		}
		filters = append(filters, f)
	}
	return filters, nil
}

func (db *DbDynamo) UpdateFilter(filter *model.Filter) error {
	// PutItem overwrites the existing item
	return db.CreateFilter(filter)
}

func (db *DbDynamo) DeleteFilter(id string) error {
	// Scan to find the filter by id since id is the range key
	result, err := db.client.Scan(context.Background(), &dynamodb.ScanInput{
		TableName:        aws.String(filtersTableName),
		FilterExpression: aws.String("id = :id"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":id": &types.AttributeValueMemberS{Value: id},
		},
		ProjectionExpression: aws.String("user_email, id"),
	})
	if err != nil {
		return err
	}
	if len(result.Items) == 0 {
		return fmt.Errorf("filter not found: %s", id)
	}
	_, err = db.client.DeleteItem(context.Background(), &dynamodb.DeleteItemInput{
		TableName: aws.String(filtersTableName),
		Key: map[string]types.AttributeValue{
			"user_email": result.Items[0]["user_email"],
			"id":         result.Items[0]["id"],
		},
	})
	return err
}

// --- Auto-reply operations ---

func (db *DbDynamo) SetAutoReply(reply *model.AutoReply) error {
	item := map[string]types.AttributeValue{
		"user_email": &types.AttributeValueMemberS{Value: reply.UserEmail},
		"enabled":    &types.AttributeValueMemberBOOL{Value: reply.Enabled},
		"subject":    &types.AttributeValueMemberS{Value: reply.Subject},
		"body":       &types.AttributeValueMemberS{Value: reply.Body},
		"start_date": &types.AttributeValueMemberS{Value: reply.StartDate.UTC().Format(time.RFC3339)},
		"end_date":   &types.AttributeValueMemberS{Value: reply.EndDate.UTC().Format(time.RFC3339)},
	}
	_, err := db.client.PutItem(context.Background(), &dynamodb.PutItemInput{
		TableName: aws.String(autoRepliesTableName),
		Item:      item,
	})
	return err
}

func (db *DbDynamo) GetAutoReply(userEmail string) (*model.AutoReply, error) {
	result, err := db.client.GetItem(context.Background(), &dynamodb.GetItemInput{
		TableName: aws.String(autoRepliesTableName),
		Key: map[string]types.AttributeValue{
			"user_email": &types.AttributeValueMemberS{Value: userEmail},
		},
	})
	if err != nil {
		return nil, err
	}
	if result.Item == nil {
		return nil, fmt.Errorf("auto-reply not found for: %s", userEmail)
	}
	r := &model.AutoReply{}
	if v, ok := result.Item["user_email"]; ok {
		r.UserEmail = v.(*types.AttributeValueMemberS).Value
	}
	if v, ok := result.Item["enabled"]; ok {
		r.Enabled = v.(*types.AttributeValueMemberBOOL).Value
	}
	if v, ok := result.Item["subject"]; ok {
		r.Subject = v.(*types.AttributeValueMemberS).Value
	}
	if v, ok := result.Item["body"]; ok {
		r.Body = v.(*types.AttributeValueMemberS).Value
	}
	if v, ok := result.Item["start_date"]; ok {
		r.StartDate, _ = time.Parse(time.RFC3339, v.(*types.AttributeValueMemberS).Value)
	}
	if v, ok := result.Item["end_date"]; ok {
		r.EndDate, _ = time.Parse(time.RFC3339, v.(*types.AttributeValueMemberS).Value)
	}
	return r, nil
}

func (db *DbDynamo) DeleteAutoReply(userEmail string) error {
	_, err := db.client.DeleteItem(context.Background(), &dynamodb.DeleteItemInput{
		TableName: aws.String(autoRepliesTableName),
		Key: map[string]types.AttributeValue{
			"user_email": &types.AttributeValueMemberS{Value: userEmail},
		},
	})
	return err
}

func (db *DbDynamo) RecordAutoReplySent(userEmail, senderEmail string) error {
	item := map[string]types.AttributeValue{
		"user_email":   &types.AttributeValueMemberS{Value: userEmail},
		"sender_email": &types.AttributeValueMemberS{Value: senderEmail},
		"sent_at":      &types.AttributeValueMemberS{Value: time.Now().UTC().Format(time.RFC3339)},
	}
	_, err := db.client.PutItem(context.Background(), &dynamodb.PutItemInput{
		TableName: aws.String(autoReplyLogTableName),
		Item:      item,
	})
	return err
}

func (db *DbDynamo) HasAutoRepliedRecently(userEmail, senderEmail string, cooldown time.Duration) bool {
	result, err := db.client.GetItem(context.Background(), &dynamodb.GetItemInput{
		TableName: aws.String(autoReplyLogTableName),
		Key: map[string]types.AttributeValue{
			"user_email":   &types.AttributeValueMemberS{Value: userEmail},
			"sender_email": &types.AttributeValueMemberS{Value: senderEmail},
		},
	})
	if err != nil || result.Item == nil {
		return false
	}
	sentAtAttr, ok := result.Item["sent_at"]
	if !ok {
		return false
	}
	sentAt, err := time.Parse(time.RFC3339, sentAtAttr.(*types.AttributeValueMemberS).Value)
	if err != nil {
		return false
	}
	return time.Since(sentAt) < cooldown
}

// --- Count operations ---

func (db *DbDynamo) CountUnread(ownerEmail, folder string) int {
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

// --- Search operations ---

func (db *DbDynamo) SearchMessages(ownerEmail, query string) ([]*model.Message, error) {
	result, err := db.client.Query(context.Background(), &dynamodb.QueryInput{
		TableName:              aws.String(messagesTableName),
		KeyConditionExpression: aws.String("owner_user = :owner"),
		FilterExpression:       aws.String("deleted = :false"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":owner": &types.AttributeValueMemberS{Value: ownerEmail},
			":false": &types.AttributeValueMemberBOOL{Value: false},
		},
		ScanIndexForward: aws.Bool(false),
	})
	if err != nil {
		return nil, err
	}
	allMsgs, err := db.unmarshalMessages(result.Items)
	if err != nil {
		return nil, err
	}
	lowerQuery := strings.ToLower(query)
	var matched []*model.Message
	for _, msg := range allMsgs {
		if strings.Contains(strings.ToLower(msg.Subject), lowerQuery) ||
			strings.Contains(strings.ToLower(msg.Body), lowerQuery) ||
			strings.Contains(strings.ToLower(msg.From), lowerQuery) ||
			strings.Contains(strings.ToLower(strings.Join(msg.To, " ")), lowerQuery) {
			matched = append(matched, msg)
		}
	}
	return matched, nil
}

// --- Folder operations ---

func (db *DbDynamo) ListUserFolders(ownerEmail string) ([]string, error) {
	result, err := db.client.Query(context.Background(), &dynamodb.QueryInput{
		TableName:              aws.String(messagesTableName),
		KeyConditionExpression: aws.String("owner_user = :owner"),
		FilterExpression:       aws.String("deleted = :false"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":owner": &types.AttributeValueMemberS{Value: ownerEmail},
			":false": &types.AttributeValueMemberBOOL{Value: false},
		},
		ProjectionExpression: aws.String("folder"),
	})
	if err != nil {
		return nil, err
	}
	seen := make(map[string]bool)
	var folders []string
	for _, item := range result.Items {
		if v, ok := item["folder"]; ok {
			folder := v.(*types.AttributeValueMemberS).Value
			if !seen[folder] {
				seen[folder] = true
				folders = append(folders, folder)
			}
		}
	}
	return folders, nil
}

// --- Contact operations ---

func (db *DbDynamo) CreateContact(contact *model.Contact) error {
	now := time.Now().UTC().Format(time.RFC3339)
	item := map[string]types.AttributeValue{
		"owner_email": &types.AttributeValueMemberS{Value: contact.OwnerEmail},
		"id":          &types.AttributeValueMemberS{Value: contact.ID},
		"vcard_data":  &types.AttributeValueMemberS{Value: contact.VCardData},
		"etag":        &types.AttributeValueMemberS{Value: contact.ETag},
		"created_at":  &types.AttributeValueMemberS{Value: now},
		"updated_at":  &types.AttributeValueMemberS{Value: now},
	}
	_, err := db.client.PutItem(context.Background(), &dynamodb.PutItemInput{
		TableName: aws.String(contactsTableName),
		Item:      item,
	})
	return err
}

func (db *DbDynamo) GetContact(id string) (*model.Contact, error) {
	// Scan since id is the range key and we don't know owner_email
	result, err := db.client.Scan(context.Background(), &dynamodb.ScanInput{
		TableName:        aws.String(contactsTableName),
		FilterExpression: aws.String("id = :id"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":id": &types.AttributeValueMemberS{Value: id},
		},
	})
	if err != nil {
		return nil, err
	}
	if len(result.Items) == 0 {
		return nil, fmt.Errorf("contact not found: %s", id)
	}
	return db.unmarshalContact(result.Items[0])
}

func (db *DbDynamo) ListContacts(ownerEmail string) ([]*model.Contact, error) {
	result, err := db.client.Query(context.Background(), &dynamodb.QueryInput{
		TableName:              aws.String(contactsTableName),
		KeyConditionExpression: aws.String("owner_email = :oe"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":oe": &types.AttributeValueMemberS{Value: ownerEmail},
		},
	})
	if err != nil {
		return nil, err
	}
	var contacts []*model.Contact
	for _, item := range result.Items {
		c, err := db.unmarshalContact(item)
		if err != nil {
			return nil, err
		}
		contacts = append(contacts, c)
	}
	return contacts, nil
}

func (db *DbDynamo) UpdateContact(contact *model.Contact) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := db.client.UpdateItem(context.Background(), &dynamodb.UpdateItemInput{
		TableName: aws.String(contactsTableName),
		Key: map[string]types.AttributeValue{
			"owner_email": &types.AttributeValueMemberS{Value: contact.OwnerEmail},
			"id":          &types.AttributeValueMemberS{Value: contact.ID},
		},
		UpdateExpression: aws.String("SET vcard_data = :vd, etag = :et, updated_at = :ua"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":vd": &types.AttributeValueMemberS{Value: contact.VCardData},
			":et": &types.AttributeValueMemberS{Value: contact.ETag},
			":ua": &types.AttributeValueMemberS{Value: now},
		},
	})
	return err
}

func (db *DbDynamo) DeleteContact(id string) error {
	// Scan to find owner_email since id is the range key
	result, err := db.client.Scan(context.Background(), &dynamodb.ScanInput{
		TableName:        aws.String(contactsTableName),
		FilterExpression: aws.String("id = :id"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":id": &types.AttributeValueMemberS{Value: id},
		},
		ProjectionExpression: aws.String("owner_email, id"),
	})
	if err != nil {
		return err
	}
	if len(result.Items) == 0 {
		return fmt.Errorf("contact not found: %s", id)
	}
	_, err = db.client.DeleteItem(context.Background(), &dynamodb.DeleteItemInput{
		TableName: aws.String(contactsTableName),
		Key: map[string]types.AttributeValue{
			"owner_email": result.Items[0]["owner_email"],
			"id":          result.Items[0]["id"],
		},
	})
	return err
}

func (db *DbDynamo) unmarshalContact(item map[string]types.AttributeValue) (*model.Contact, error) {
	c := &model.Contact{}
	if v, ok := item["id"]; ok {
		c.ID = v.(*types.AttributeValueMemberS).Value
	}
	if v, ok := item["owner_email"]; ok {
		c.OwnerEmail = v.(*types.AttributeValueMemberS).Value
	}
	if v, ok := item["vcard_data"]; ok {
		c.VCardData = v.(*types.AttributeValueMemberS).Value
	}
	if v, ok := item["etag"]; ok {
		c.ETag = v.(*types.AttributeValueMemberS).Value
	}
	if v, ok := item["created_at"]; ok {
		c.CreatedAt, _ = time.Parse(time.RFC3339, v.(*types.AttributeValueMemberS).Value)
	}
	if v, ok := item["updated_at"]; ok {
		c.UpdatedAt, _ = time.Parse(time.RFC3339, v.(*types.AttributeValueMemberS).Value)
	}
	return c, nil
}

// --- Attachment operations (stub for DynamoDB) ---

func (db *DbDynamo) SaveAttachment(att *model.Attachment) error {
	return fmt.Errorf("attachment store not yet implemented for DynamoDB")
}
func (db *DbDynamo) ListAttachments(mailContentID string) ([]model.Attachment, error) {
	return nil, nil
}
func (db *DbDynamo) GetAttachment(id string) (*model.Attachment, error) {
	return nil, fmt.Errorf("attachment not found")
}
func (db *DbDynamo) DeleteAttachmentsByMessage(mailContentID string) error { return nil }

// --- Auth / 2FA operations (stub — full implementation needed for DynamoDB) ---

func (db *DbDynamo) Enable2FA(email, secret, backupCodes string) error {
	return fmt.Errorf("2FA not yet implemented for DynamoDB")
}
func (db *DbDynamo) Disable2FA(email string) error {
	return fmt.Errorf("2FA not yet implemented for DynamoDB")
}
func (db *DbDynamo) Get2FAStatus(email string) (bool, string, string, error) {
	return false, "", "", nil // 2FA not enabled by default
}
func (db *DbDynamo) CreateTrustedDevice(device *model.TrustedDevice) error {
	return fmt.Errorf("trusted devices not yet implemented for DynamoDB")
}
func (db *DbDynamo) IsTrustedDevice(email, fingerprint string) (bool, error) { return false, nil }
func (db *DbDynamo) ListTrustedDevices(email string) ([]*model.TrustedDevice, error) {
	return nil, nil
}
func (db *DbDynamo) RevokeTrustedDevice(id string) error { return nil }
func (db *DbDynamo) UpdateDeviceLastSeen(id string) error { return nil }
func (db *DbDynamo) CreateOTP(otp *model.OTP) error {
	return fmt.Errorf("OTP not yet implemented for DynamoDB")
}
func (db *DbDynamo) GetOTP(email string) (*model.OTP, error) { return nil, fmt.Errorf("not found") }
func (db *DbDynamo) IncrementOTPAttempts(email string) error { return nil }
func (db *DbDynamo) ClearOTP(email string) error            { return nil }
func (db *DbDynamo) CreateLoginToken(token *model.LoginToken) error {
	return fmt.Errorf("login tokens not yet implemented for DynamoDB")
}
func (db *DbDynamo) GetLoginToken(token string) (*model.LoginToken, error) {
	return nil, fmt.Errorf("not found")
}
func (db *DbDynamo) DeleteLoginToken(token string) error { return nil }

// --- Domain operations ---

func (db *DbDynamo) CreateDomain(domain *model.Domain) error {
	now := time.Now().UTC().Format(time.RFC3339)
	item := map[string]types.AttributeValue{
		"name":         &types.AttributeValueMemberS{Value: domain.Name},
		"api_key_hash": &types.AttributeValueMemberS{Value: domain.APIKeyHash},
		"ses_status":   &types.AttributeValueMemberS{Value: domain.SESStatus},
		"dkim_status":  &types.AttributeValueMemberS{Value: domain.DKIMStatus},
		"status":       &types.AttributeValueMemberS{Value: domain.Status},
		"created_by":   &types.AttributeValueMemberS{Value: domain.CreatedBy},
		"created_at":   &types.AttributeValueMemberS{Value: now},
	}
	_, err := db.client.PutItem(context.Background(), &dynamodb.PutItemInput{
		TableName: aws.String(domainsTableName),
		Item:      item,
	})
	return err
}

func (db *DbDynamo) GetDomain(name string) (*model.Domain, error) {
	result, err := db.client.GetItem(context.Background(), &dynamodb.GetItemInput{
		TableName: aws.String(domainsTableName),
		Key:       map[string]types.AttributeValue{"name": &types.AttributeValueMemberS{Value: name}},
	})
	if err != nil {
		return nil, err
	}
	if result.Item == nil {
		return nil, fmt.Errorf("domain not found: %s", name)
	}
	return unmarshalDomain(result.Item), nil
}

func (db *DbDynamo) ListDomains() ([]*model.Domain, error) {
	result, err := db.client.Scan(context.Background(), &dynamodb.ScanInput{
		TableName: aws.String(domainsTableName),
	})
	if err != nil {
		return nil, err
	}
	var domains []*model.Domain
	for _, item := range result.Items {
		domains = append(domains, unmarshalDomain(item))
	}
	return domains, nil
}

func (db *DbDynamo) UpdateDomainStatus(name, sesStatus, dkimStatus string) error {
	_, err := db.client.UpdateItem(context.Background(), &dynamodb.UpdateItemInput{
		TableName: aws.String(domainsTableName),
		Key:       map[string]types.AttributeValue{"name": &types.AttributeValueMemberS{Value: name}},
		UpdateExpression: aws.String("SET ses_status = :ss, dkim_status = :ds"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":ss": &types.AttributeValueMemberS{Value: sesStatus},
			":ds": &types.AttributeValueMemberS{Value: dkimStatus},
		},
	})
	return err
}

func (db *DbDynamo) DeleteDomain(name string) error {
	_, err := db.client.DeleteItem(context.Background(), &dynamodb.DeleteItemInput{
		TableName: aws.String(domainsTableName),
		Key:       map[string]types.AttributeValue{"name": &types.AttributeValueMemberS{Value: name}},
	})
	return err
}

func unmarshalDomain(item map[string]types.AttributeValue) *model.Domain {
	d := &model.Domain{}
	if v, ok := item["name"]; ok {
		d.Name = v.(*types.AttributeValueMemberS).Value
	}
	if v, ok := item["api_key_hash"]; ok {
		d.APIKeyHash = v.(*types.AttributeValueMemberS).Value
	}
	if v, ok := item["ses_status"]; ok {
		d.SESStatus = v.(*types.AttributeValueMemberS).Value
	}
	if v, ok := item["dkim_status"]; ok {
		d.DKIMStatus = v.(*types.AttributeValueMemberS).Value
	}
	if v, ok := item["status"]; ok {
		d.Status = v.(*types.AttributeValueMemberS).Value
	}
	if v, ok := item["created_by"]; ok {
		d.CreatedBy = v.(*types.AttributeValueMemberS).Value
	}
	if v, ok := item["created_at"]; ok {
		d.CreatedAt, _ = time.Parse(time.RFC3339, v.(*types.AttributeValueMemberS).Value)
	}
	return d
}

// --- OAuth operations ---

func (db *DbDynamo) CreateOAuthClient(client *model.OAuthClient) error {
	now := time.Now().UTC().Format(time.RFC3339)
	item := map[string]types.AttributeValue{
		"domain":       &types.AttributeValueMemberS{Value: client.Domain},
		"id":           &types.AttributeValueMemberS{Value: client.ID},
		"name":         &types.AttributeValueMemberS{Value: client.Name},
		"client_id":    &types.AttributeValueMemberS{Value: client.ClientID},
		"secret_hash":  &types.AttributeValueMemberS{Value: client.SecretHash},
		"redirect_uri": &types.AttributeValueMemberS{Value: client.RedirectURI},
		"created_by":   &types.AttributeValueMemberS{Value: client.CreatedBy},
		"created_at":   &types.AttributeValueMemberS{Value: now},
	}
	_, err := db.client.PutItem(context.Background(), &dynamodb.PutItemInput{
		TableName: aws.String(oauthClientsTableName),
		Item:      item,
	})
	return err
}

func (db *DbDynamo) GetOAuthClient(clientID string) (*model.OAuthClient, error) {
	// Use GSI on client_id
	result, err := db.client.Query(context.Background(), &dynamodb.QueryInput{
		TableName:              aws.String(oauthClientsTableName),
		IndexName:              aws.String("client-id-index"),
		KeyConditionExpression: aws.String("client_id = :cid"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":cid": &types.AttributeValueMemberS{Value: clientID},
		},
		Limit: aws.Int32(1),
	})
	if err != nil {
		return nil, err
	}
	if len(result.Items) == 0 {
		return nil, fmt.Errorf("oauth client not found: %s", clientID)
	}
	return unmarshalOAuthClient(result.Items[0]), nil
}

func (db *DbDynamo) ListOAuthClients(domain string) ([]*model.OAuthClient, error) {
	result, err := db.client.Query(context.Background(), &dynamodb.QueryInput{
		TableName:              aws.String(oauthClientsTableName),
		KeyConditionExpression: aws.String("domain = :d"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":d": &types.AttributeValueMemberS{Value: domain},
		},
		ScanIndexForward: aws.Bool(false),
	})
	if err != nil {
		return nil, err
	}
	var clients []*model.OAuthClient
	for _, item := range result.Items {
		clients = append(clients, unmarshalOAuthClient(item))
	}
	return clients, nil
}

func (db *DbDynamo) DeleteOAuthClient(id string) error {
	// Scan to find domain (partition key) since we only have id (sort key)
	result, err := db.client.Scan(context.Background(), &dynamodb.ScanInput{
		TableName:            aws.String(oauthClientsTableName),
		FilterExpression:     aws.String("id = :id"),
		ExpressionAttributeValues: map[string]types.AttributeValue{":id": &types.AttributeValueMemberS{Value: id}},
		ProjectionExpression: aws.String("#d, id"),
		ExpressionAttributeNames: map[string]string{"#d": "domain"},
	})
	if err != nil || len(result.Items) == 0 {
		return fmt.Errorf("oauth client not found: %s", id)
	}
	_, err = db.client.DeleteItem(context.Background(), &dynamodb.DeleteItemInput{
		TableName: aws.String(oauthClientsTableName),
		Key: map[string]types.AttributeValue{
			"domain": result.Items[0]["domain"],
			"id":     result.Items[0]["id"],
		},
	})
	return err
}

func unmarshalOAuthClient(item map[string]types.AttributeValue) *model.OAuthClient {
	c := &model.OAuthClient{}
	if v, ok := item["id"]; ok {
		c.ID = v.(*types.AttributeValueMemberS).Value
	}
	if v, ok := item["name"]; ok {
		c.Name = v.(*types.AttributeValueMemberS).Value
	}
	if v, ok := item["client_id"]; ok {
		c.ClientID = v.(*types.AttributeValueMemberS).Value
	}
	if v, ok := item["secret_hash"]; ok {
		c.SecretHash = v.(*types.AttributeValueMemberS).Value
	}
	if v, ok := item["redirect_uri"]; ok {
		c.RedirectURI = v.(*types.AttributeValueMemberS).Value
	}
	if v, ok := item["domain"]; ok {
		c.Domain = v.(*types.AttributeValueMemberS).Value
	}
	if v, ok := item["created_by"]; ok {
		c.CreatedBy = v.(*types.AttributeValueMemberS).Value
	}
	if v, ok := item["created_at"]; ok {
		c.CreatedAt, _ = time.Parse(time.RFC3339, v.(*types.AttributeValueMemberS).Value)
	}
	return c
}

func (db *DbDynamo) CreateOAuthCode(code *model.OAuthCode) error {
	item := map[string]types.AttributeValue{
		"code":         &types.AttributeValueMemberS{Value: code.Code},
		"client_id":    &types.AttributeValueMemberS{Value: code.ClientID},
		"user_email":   &types.AttributeValueMemberS{Value: code.UserEmail},
		"redirect_uri": &types.AttributeValueMemberS{Value: code.RedirectURI},
		"scope":        &types.AttributeValueMemberS{Value: code.Scope},
		"nonce":        &types.AttributeValueMemberS{Value: code.Nonce},
		"expires_at":   &types.AttributeValueMemberS{Value: code.ExpiresAt.UTC().Format(time.RFC3339)},
		"used":         &types.AttributeValueMemberBOOL{Value: false},
	}
	_, err := db.client.PutItem(context.Background(), &dynamodb.PutItemInput{
		TableName: aws.String(oauthCodesTableName),
		Item:      item,
	})
	return err
}

func (db *DbDynamo) GetOAuthCode(code string) (*model.OAuthCode, error) {
	result, err := db.client.GetItem(context.Background(), &dynamodb.GetItemInput{
		TableName: aws.String(oauthCodesTableName),
		Key:       map[string]types.AttributeValue{"code": &types.AttributeValueMemberS{Value: code}},
	})
	if err != nil {
		return nil, err
	}
	if result.Item == nil {
		return nil, fmt.Errorf("oauth code not found")
	}
	c := &model.OAuthCode{}
	if v, ok := result.Item["code"]; ok {
		c.Code = v.(*types.AttributeValueMemberS).Value
	}
	if v, ok := result.Item["client_id"]; ok {
		c.ClientID = v.(*types.AttributeValueMemberS).Value
	}
	if v, ok := result.Item["user_email"]; ok {
		c.UserEmail = v.(*types.AttributeValueMemberS).Value
	}
	if v, ok := result.Item["redirect_uri"]; ok {
		c.RedirectURI = v.(*types.AttributeValueMemberS).Value
	}
	if v, ok := result.Item["scope"]; ok {
		c.Scope = v.(*types.AttributeValueMemberS).Value
	}
	if v, ok := result.Item["nonce"]; ok {
		c.Nonce = v.(*types.AttributeValueMemberS).Value
	}
	if v, ok := result.Item["expires_at"]; ok {
		c.ExpiresAt, _ = time.Parse(time.RFC3339, v.(*types.AttributeValueMemberS).Value)
	}
	if v, ok := result.Item["used"]; ok {
		c.Used = v.(*types.AttributeValueMemberBOOL).Value
	}
	return c, nil
}

func (db *DbDynamo) MarkOAuthCodeUsed(code string) error {
	_, err := db.client.UpdateItem(context.Background(), &dynamodb.UpdateItemInput{
		TableName: aws.String(oauthCodesTableName),
		Key:       map[string]types.AttributeValue{"code": &types.AttributeValueMemberS{Value: code}},
		UpdateExpression: aws.String("SET used = :t"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":t": &types.AttributeValueMemberBOOL{Value: true},
		},
	})
	return err
}

func (db *DbDynamo) CreateOAuthToken(token *model.OAuthToken) error {
	item := map[string]types.AttributeValue{
		"token":      &types.AttributeValueMemberS{Value: token.Token},
		"client_id":  &types.AttributeValueMemberS{Value: token.ClientID},
		"user_email": &types.AttributeValueMemberS{Value: token.UserEmail},
		"scope":      &types.AttributeValueMemberS{Value: token.Scope},
		"expires_at": &types.AttributeValueMemberS{Value: token.ExpiresAt.UTC().Format(time.RFC3339)},
	}
	_, err := db.client.PutItem(context.Background(), &dynamodb.PutItemInput{
		TableName: aws.String(oauthTokensTableName),
		Item:      item,
	})
	return err
}

func (db *DbDynamo) GetOAuthToken(token string) (*model.OAuthToken, error) {
	result, err := db.client.GetItem(context.Background(), &dynamodb.GetItemInput{
		TableName: aws.String(oauthTokensTableName),
		Key:       map[string]types.AttributeValue{"token": &types.AttributeValueMemberS{Value: token}},
	})
	if err != nil {
		return nil, err
	}
	if result.Item == nil {
		return nil, fmt.Errorf("oauth token not found")
	}
	t := &model.OAuthToken{}
	if v, ok := result.Item["token"]; ok {
		t.Token = v.(*types.AttributeValueMemberS).Value
	}
	if v, ok := result.Item["client_id"]; ok {
		t.ClientID = v.(*types.AttributeValueMemberS).Value
	}
	if v, ok := result.Item["user_email"]; ok {
		t.UserEmail = v.(*types.AttributeValueMemberS).Value
	}
	if v, ok := result.Item["scope"]; ok {
		t.Scope = v.(*types.AttributeValueMemberS).Value
	}
	if v, ok := result.Item["expires_at"]; ok {
		t.ExpiresAt, _ = time.Parse(time.RFC3339, v.(*types.AttributeValueMemberS).Value)
	}
	return t, nil
}

func (db *DbDynamo) DeleteOAuthToken(token string) error {
	_, err := db.client.DeleteItem(context.Background(), &dynamodb.DeleteItemInput{
		TableName: aws.String(oauthTokensTableName),
		Key:       map[string]types.AttributeValue{"token": &types.AttributeValueMemberS{Value: token}},
	})
	return err
}
