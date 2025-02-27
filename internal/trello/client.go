package trello

import (
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/adlio/trello"
	"github.com/dolaszy/glue/pkg/models"
)

// Client handles interactions with the Trello API
type Client struct {
	client *trello.Client
}

// NewClient creates a new Trello client
func NewClient() *Client {
	// Get Trello API key and token from environment variables
	key := os.Getenv("TRELLO_API_KEY")
	token := os.Getenv("TRELLO_TOKEN")

	if key == "" || token == "" {
		log.Println("Warning: TRELLO_API_KEY and/or TRELLO_TOKEN environment variables not set")
	}

	// Create Trello client
	client := trello.NewClient(key, token)

	return &Client{
		client: client,
	}
}

// CreateCard creates a Trello card from a GitHub issue
func (c *Client) CreateCard(boardName string, issue models.GitHubIssue) (string, error) {
	// Find the board by name
	member, err := c.client.GetMember("me", trello.Defaults())
	if err != nil {
		return "", fmt.Errorf("failed to fetch Trello member: %v", err)
	}

	boards, err := member.GetBoards(trello.Defaults())
	if err != nil {
		return "", fmt.Errorf("failed to fetch Trello boards: %v", err)
	}

	var board *trello.Board
	for _, b := range boards {
		if strings.EqualFold(b.Name, boardName) {
			board = b
			break
		}
	}

	if board == nil {
		return "", fmt.Errorf("board '%s' not found", boardName)
	}

	// Get the lists on the board
	lists, err := board.GetLists(trello.Defaults())
	if err != nil {
		return "", fmt.Errorf("failed to fetch lists for board '%s': %v", boardName, err)
	}

	// Find appropriate list (To Do list for new cards)
	var list *trello.List
	for _, l := range lists {
		if strings.EqualFold(l.Name, "To Do") || strings.EqualFold(l.Name, "Backlog") {
			list = l
			break
		}
	}

	// If no appropriate list found, use the first list
	if list == nil && len(lists) > 0 {
		list = lists[0]
	} else if list == nil {
		// Create a new list if none exist
		newList, err := board.CreateList("To Do", trello.Defaults())
		if err != nil {
			return "", fmt.Errorf("failed to create 'To Do' list: %v", err)
		}

		list = newList
	}

	// Prepare card description
	description := fmt.Sprintf("%s\n\n---\n*Created by glue from GitHub issue #%d*",
		issue.Description, issue.Number)

	// Create card
	card := &trello.Card{
		Name:   issue.Title,
		Desc:   description,
		IDList: list.ID,
	}

	err = c.client.CreateCard(card, trello.Defaults())
	if err != nil {
		return "", fmt.Errorf("failed to create Trello card: %v", err)
	}

	return card.ID, nil
}

// GetBoardStats returns statistics about a Trello board
func (c *Client) GetBoardStats(boardName string) (int, int, error) {
	// Find the board by name
	member, err := c.client.GetMember("me", trello.Defaults())
	if err != nil {
		return 0, 0, fmt.Errorf("failed to fetch Trello member: %v", err)
	}

	boards, err := member.GetBoards(trello.Defaults())
	if err != nil {
		return 0, 0, fmt.Errorf("failed to fetch Trello boards: %v", err)
	}

	var board *trello.Board
	for _, b := range boards {
		if strings.EqualFold(b.Name, boardName) {
			board = b
			break
		}
	}

	if board == nil {
		return 0, 0, fmt.Errorf("board '%s' not found", boardName)
	}

	// Get all cards on the board
	cards, err := board.GetCards(trello.Defaults())
	if err != nil {
		return 0, 0, fmt.Errorf("failed to fetch cards for board '%s': %v", boardName, err)
	}

	// Count cards
	totalCards := len(cards)
	gluedCards := 0

	for _, card := range cards {
		// Check if the card was created by glue (look for the signature in description)
		if strings.Contains(card.Desc, "Created by glue from GitHub issue") {
			gluedCards++
		}
	}

	return totalCards, gluedCards, nil
}
