//go:build ignore

// Standalone script to create DynamoDB tables for bdsmail.
// Run: go run sql/init_dynamodb.go --region us-east-1
package main

import (
	"context"
	"flag"
	"fmt"
	"log"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

var region = flag.String("region", "us-east-1", "AWS region")

func main() {
	flag.Parse()
	ctx := context.Background()

	cfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(*region))
	if err != nil {
		log.Fatalf("AWS config failed: %v", err)
	}
	client := dynamodb.NewFromConfig(cfg)

	tables := []struct {
		name    string
		keys    []types.KeySchemaElement
		attrs   []types.AttributeDefinition
		gsis    []types.GlobalSecondaryIndex
	}{
		{
			name:  "bdsmail-users",
			keys:  []types.KeySchemaElement{{AttributeName: aws.String("email"), KeyType: types.KeyTypeHash}},
			attrs: []types.AttributeDefinition{{AttributeName: aws.String("email"), AttributeType: types.ScalarAttributeTypeS}},
		},
		{
			name: "bdsmail-messages",
			keys: []types.KeySchemaElement{
				{AttributeName: aws.String("owner_user"), KeyType: types.KeyTypeHash},
				{AttributeName: aws.String("sort_key"), KeyType: types.KeyTypeRange},
			},
			attrs: []types.AttributeDefinition{
				{AttributeName: aws.String("owner_user"), AttributeType: types.ScalarAttributeTypeS},
				{AttributeName: aws.String("sort_key"), AttributeType: types.ScalarAttributeTypeS},
				{AttributeName: aws.String("folder"), AttributeType: types.ScalarAttributeTypeS},
				{AttributeName: aws.String("id"), AttributeType: types.ScalarAttributeTypeS},
			},
			gsis: []types.GlobalSecondaryIndex{
				{IndexName: aws.String("folder-index"), KeySchema: []types.KeySchemaElement{
					{AttributeName: aws.String("owner_user"), KeyType: types.KeyTypeHash},
					{AttributeName: aws.String("folder"), KeyType: types.KeyTypeRange},
				}, Projection: &types.Projection{ProjectionType: types.ProjectionTypeAll}},
				{IndexName: aws.String("id-index"), KeySchema: []types.KeySchemaElement{
					{AttributeName: aws.String("id"), KeyType: types.KeyTypeHash},
				}, Projection: &types.Projection{ProjectionType: types.ProjectionTypeAll}},
			},
		},
		{
			name:  "bdsmail-aliases",
			keys:  []types.KeySchemaElement{{AttributeName: aws.String("alias_email"), KeyType: types.KeyTypeHash}},
			attrs: []types.AttributeDefinition{{AttributeName: aws.String("alias_email"), AttributeType: types.ScalarAttributeTypeS}},
		},
		{
			name:  "bdsmail-mailing-lists",
			keys:  []types.KeySchemaElement{{AttributeName: aws.String("list_address"), KeyType: types.KeyTypeHash}},
			attrs: []types.AttributeDefinition{{AttributeName: aws.String("list_address"), AttributeType: types.ScalarAttributeTypeS}},
		},
		{
			name: "bdsmail-list-members",
			keys: []types.KeySchemaElement{
				{AttributeName: aws.String("list_address"), KeyType: types.KeyTypeHash},
				{AttributeName: aws.String("member_email"), KeyType: types.KeyTypeRange},
			},
			attrs: []types.AttributeDefinition{
				{AttributeName: aws.String("list_address"), AttributeType: types.ScalarAttributeTypeS},
				{AttributeName: aws.String("member_email"), AttributeType: types.ScalarAttributeTypeS},
			},
		},
		{
			name: "bdsmail-filters",
			keys: []types.KeySchemaElement{
				{AttributeName: aws.String("user_email"), KeyType: types.KeyTypeHash},
				{AttributeName: aws.String("id"), KeyType: types.KeyTypeRange},
			},
			attrs: []types.AttributeDefinition{
				{AttributeName: aws.String("user_email"), AttributeType: types.ScalarAttributeTypeS},
				{AttributeName: aws.String("id"), AttributeType: types.ScalarAttributeTypeS},
			},
		},
		{
			name:  "bdsmail-auto-replies",
			keys:  []types.KeySchemaElement{{AttributeName: aws.String("user_email"), KeyType: types.KeyTypeHash}},
			attrs: []types.AttributeDefinition{{AttributeName: aws.String("user_email"), AttributeType: types.ScalarAttributeTypeS}},
		},
		{
			name: "bdsmail-auto-reply-log",
			keys: []types.KeySchemaElement{
				{AttributeName: aws.String("user_email"), KeyType: types.KeyTypeHash},
				{AttributeName: aws.String("sender_email"), KeyType: types.KeyTypeRange},
			},
			attrs: []types.AttributeDefinition{
				{AttributeName: aws.String("user_email"), AttributeType: types.ScalarAttributeTypeS},
				{AttributeName: aws.String("sender_email"), AttributeType: types.ScalarAttributeTypeS},
			},
		},
		{
			name: "bdsmail-contacts",
			keys: []types.KeySchemaElement{
				{AttributeName: aws.String("owner_email"), KeyType: types.KeyTypeHash},
				{AttributeName: aws.String("id"), KeyType: types.KeyTypeRange},
			},
			attrs: []types.AttributeDefinition{
				{AttributeName: aws.String("owner_email"), AttributeType: types.ScalarAttributeTypeS},
				{AttributeName: aws.String("id"), AttributeType: types.ScalarAttributeTypeS},
			},
		},
		{
			name:  "bdsmail-domains",
			keys:  []types.KeySchemaElement{{AttributeName: aws.String("name"), KeyType: types.KeyTypeHash}},
			attrs: []types.AttributeDefinition{{AttributeName: aws.String("name"), AttributeType: types.ScalarAttributeTypeS}},
		},
		{
			name: "bdsmail-oauth-clients",
			keys: []types.KeySchemaElement{
				{AttributeName: aws.String("domain"), KeyType: types.KeyTypeHash},
				{AttributeName: aws.String("id"), KeyType: types.KeyTypeRange},
			},
			attrs: []types.AttributeDefinition{
				{AttributeName: aws.String("domain"), AttributeType: types.ScalarAttributeTypeS},
				{AttributeName: aws.String("id"), AttributeType: types.ScalarAttributeTypeS},
				{AttributeName: aws.String("client_id"), AttributeType: types.ScalarAttributeTypeS},
			},
			gsis: []types.GlobalSecondaryIndex{
				{IndexName: aws.String("client-id-index"), KeySchema: []types.KeySchemaElement{
					{AttributeName: aws.String("client_id"), KeyType: types.KeyTypeHash},
				}, Projection: &types.Projection{ProjectionType: types.ProjectionTypeAll}},
			},
		},
		{
			name:  "bdsmail-oauth-codes",
			keys:  []types.KeySchemaElement{{AttributeName: aws.String("code"), KeyType: types.KeyTypeHash}},
			attrs: []types.AttributeDefinition{{AttributeName: aws.String("code"), AttributeType: types.ScalarAttributeTypeS}},
		},
		{
			name:  "bdsmail-oauth-tokens",
			keys:  []types.KeySchemaElement{{AttributeName: aws.String("token"), KeyType: types.KeyTypeHash}},
			attrs: []types.AttributeDefinition{{AttributeName: aws.String("token"), AttributeType: types.ScalarAttributeTypeS}},
		},
	}

	for _, t := range tables {
		_, err := client.DescribeTable(ctx, &dynamodb.DescribeTableInput{TableName: aws.String(t.name)})
		if err == nil {
			fmt.Printf("  EXISTS: %s\n", t.name)
			continue
		}
		input := &dynamodb.CreateTableInput{
			TableName:            aws.String(t.name),
			KeySchema:            t.keys,
			AttributeDefinitions: t.attrs,
			BillingMode:          types.BillingModePayPerRequest,
		}
		if len(t.gsis) > 0 {
			input.GlobalSecondaryIndexes = t.gsis
		}
		if _, err := client.CreateTable(ctx, input); err != nil {
			log.Printf("  FAILED: %s: %v", t.name, err)
		} else {
			fmt.Printf("  CREATED: %s\n", t.name)
		}
	}
	fmt.Println("DynamoDB initialization complete.")
}
