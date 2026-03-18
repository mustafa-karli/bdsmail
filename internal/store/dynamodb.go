package store

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/mustafakarli/bdsmail/internal/model"
)

const (
	usersTableName    = "bdsmail-users"
	messagesTableName = "bdsmail-messages"
	folderIndexName   = "folder-index"
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
