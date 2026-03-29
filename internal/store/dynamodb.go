package store

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
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
	if err := db.ensureTables(); err != nil {
		return nil, err
	}
	return db, nil
}

func (db *DbDynamo) Close() error {
	return nil
}

func (db *DbDynamo) GetQueries() map[string]string {
	return nil // DynamoDB does not use SQL queries
}

func (db *DbDynamo) ensureTables() error {
	ctx := context.Background()

	// Create users table
	if err := db.createTableIfNotExists(ctx, usersTableName, []types.KeySchemaElement{
		{AttributeName: aws.String("email"), KeyType: types.KeyTypeHash},
	}, []types.AttributeDefinition{
		{AttributeName: aws.String("email"), AttributeType: types.ScalarAttributeTypeS},
	}, nil); err != nil {
		return err
	}

	// Create messages table with GSI for folder queries
	if err := db.createTableIfNotExists(ctx, messagesTableName, []types.KeySchemaElement{
		{AttributeName: aws.String("owner_user"), KeyType: types.KeyTypeHash},
		{AttributeName: aws.String("sort_key"), KeyType: types.KeyTypeRange},
	}, []types.AttributeDefinition{
		{AttributeName: aws.String("owner_user"), AttributeType: types.ScalarAttributeTypeS},
		{AttributeName: aws.String("sort_key"), AttributeType: types.ScalarAttributeTypeS},
		{AttributeName: aws.String("folder"), AttributeType: types.ScalarAttributeTypeS},
		{AttributeName: aws.String("id"), AttributeType: types.ScalarAttributeTypeS},
	}, []types.GlobalSecondaryIndex{
		{
			IndexName: aws.String(folderIndexName),
			KeySchema: []types.KeySchemaElement{
				{AttributeName: aws.String("owner_user"), KeyType: types.KeyTypeHash},
				{AttributeName: aws.String("folder"), KeyType: types.KeyTypeRange},
			},
			Projection: &types.Projection{ProjectionType: types.ProjectionTypeAll},
		},
		{
			IndexName: aws.String("id-index"),
			KeySchema: []types.KeySchemaElement{
				{AttributeName: aws.String("id"), KeyType: types.KeyTypeHash},
			},
			Projection: &types.Projection{ProjectionType: types.ProjectionTypeAll},
		},
	}); err != nil {
		return err
	}

	// Create aliases table
	if err := db.createTableIfNotExists(ctx, aliasesTableName, []types.KeySchemaElement{
		{AttributeName: aws.String("alias_email"), KeyType: types.KeyTypeHash},
	}, []types.AttributeDefinition{
		{AttributeName: aws.String("alias_email"), AttributeType: types.ScalarAttributeTypeS},
	}, nil); err != nil {
		return err
	}

	// Create mailing lists table
	if err := db.createTableIfNotExists(ctx, mailingListsTableName, []types.KeySchemaElement{
		{AttributeName: aws.String("list_address"), KeyType: types.KeyTypeHash},
	}, []types.AttributeDefinition{
		{AttributeName: aws.String("list_address"), AttributeType: types.ScalarAttributeTypeS},
	}, nil); err != nil {
		return err
	}

	// Create list members table
	if err := db.createTableIfNotExists(ctx, listMembersTableName, []types.KeySchemaElement{
		{AttributeName: aws.String("list_address"), KeyType: types.KeyTypeHash},
		{AttributeName: aws.String("member_email"), KeyType: types.KeyTypeRange},
	}, []types.AttributeDefinition{
		{AttributeName: aws.String("list_address"), AttributeType: types.ScalarAttributeTypeS},
		{AttributeName: aws.String("member_email"), AttributeType: types.ScalarAttributeTypeS},
	}, nil); err != nil {
		return err
	}

	// Create filters table
	if err := db.createTableIfNotExists(ctx, filtersTableName, []types.KeySchemaElement{
		{AttributeName: aws.String("user_email"), KeyType: types.KeyTypeHash},
		{AttributeName: aws.String("id"), KeyType: types.KeyTypeRange},
	}, []types.AttributeDefinition{
		{AttributeName: aws.String("user_email"), AttributeType: types.ScalarAttributeTypeS},
		{AttributeName: aws.String("id"), AttributeType: types.ScalarAttributeTypeS},
	}, nil); err != nil {
		return err
	}

	// Create auto-replies table
	if err := db.createTableIfNotExists(ctx, autoRepliesTableName, []types.KeySchemaElement{
		{AttributeName: aws.String("user_email"), KeyType: types.KeyTypeHash},
	}, []types.AttributeDefinition{
		{AttributeName: aws.String("user_email"), AttributeType: types.ScalarAttributeTypeS},
	}, nil); err != nil {
		return err
	}

	// Create auto-reply log table
	if err := db.createTableIfNotExists(ctx, autoReplyLogTableName, []types.KeySchemaElement{
		{AttributeName: aws.String("user_email"), KeyType: types.KeyTypeHash},
		{AttributeName: aws.String("sender_email"), KeyType: types.KeyTypeRange},
	}, []types.AttributeDefinition{
		{AttributeName: aws.String("user_email"), AttributeType: types.ScalarAttributeTypeS},
		{AttributeName: aws.String("sender_email"), AttributeType: types.ScalarAttributeTypeS},
	}, nil); err != nil {
		return err
	}

	// Create contacts table
	if err := db.createTableIfNotExists(ctx, contactsTableName, []types.KeySchemaElement{
		{AttributeName: aws.String("owner_email"), KeyType: types.KeyTypeHash},
		{AttributeName: aws.String("id"), KeyType: types.KeyTypeRange},
	}, []types.AttributeDefinition{
		{AttributeName: aws.String("owner_email"), AttributeType: types.ScalarAttributeTypeS},
		{AttributeName: aws.String("id"), AttributeType: types.ScalarAttributeTypeS},
	}, nil); err != nil {
		return err
	}

	return nil
}

func (db *DbDynamo) createTableIfNotExists(ctx context.Context, tableName string, keySchema []types.KeySchemaElement, attrDefs []types.AttributeDefinition, gsis []types.GlobalSecondaryIndex) error {
	_, err := db.client.DescribeTable(ctx, &dynamodb.DescribeTableInput{
		TableName: aws.String(tableName),
	})
	if err == nil {
		return nil // table exists
	}

	input := &dynamodb.CreateTableInput{
		TableName:            aws.String(tableName),
		KeySchema:            keySchema,
		AttributeDefinitions: attrDefs,
		BillingMode:          types.BillingModePayPerRequest,
	}
	if len(gsis) > 0 {
		input.GlobalSecondaryIndexes = gsis
	}

	_, err = db.client.CreateTable(ctx, input)
	if err != nil {
		return fmt.Errorf("failed to create table %s: %w", tableName, err)
	}
	log.Printf("Created DynamoDB table: %s", tableName)
	return nil
}

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
		Attachments: db.MarshalAttachments(msg.Attachments),
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
