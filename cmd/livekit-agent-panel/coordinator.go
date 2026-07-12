package main

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"
)

// Coordinator orchestrates turn-taking between panelists.
type Coordinator struct {
	Panelists      []*Panelist
	Transcript     *Transcript
	Topic          string
	startingOffset int // Rotates which panelist speaks first

	mu sync.Mutex
}

// NewCoordinator creates a new coordinator for the given panelists.
func NewCoordinator(panelists []*Panelist, topic string) *Coordinator {
	return &Coordinator{
		Panelists:      panelists,
		Transcript:     NewTranscript(),
		Topic:          topic,
		startingOffset: 0,
	}
}

// OnModeratorSpeech handles a new question/prompt from the moderator.
// It triggers a round of responses from all panelists.
func (c *Coordinator) OnModeratorSpeech(ctx context.Context, text string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Add moderator's speech to transcript
	c.Transcript.Add("Moderator", text)

	fmt.Printf("\n[Moderator]: \"%s\"\n\n", text)

	// Get speaking order for this round
	order := c.selectSpeakingOrder()

	// Each panelist responds in turn
	for i, panelist := range order {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		fmt.Printf("[%s] Thinking...\n", panelist.Config.Name)

		// Generate response with current transcript context
		// Use last 10 entries to keep context manageable
		transcriptContext := c.Transcript.LastN(10)
		response, err := panelist.GenerateResponse(ctx, c.Topic, transcriptContext, text)
		if err != nil {
			log.Printf("Error generating response for %s: %v", panelist.Config.Name, err)
			continue
		}

		fmt.Printf("[%s]: \"%s\"\n", panelist.Config.Name, response)

		// Add response to transcript before speaking
		c.Transcript.Add(panelist.Config.Name, response)

		// Speak the response
		fmt.Printf("[%s] Speaking...\n", panelist.Config.Name)
		if err := panelist.Speak(ctx, response); err != nil {
			log.Printf("Error speaking for %s: %v", panelist.Config.Name, err)
		}

		// Pause between panelists (except after the last one)
		if i < len(order)-1 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(1500 * time.Millisecond):
			}
		}
	}

	// Rotate starting position for next round
	c.startingOffset = (c.startingOffset + 1) % len(c.Panelists)

	fmt.Println()
	fmt.Println("--- Ready for next question ---")
	fmt.Println()
	return nil
}

// selectSpeakingOrder returns panelists in order for the current round.
// The starting position rotates each round for variety.
func (c *Coordinator) selectSpeakingOrder() []*Panelist {
	n := len(c.Panelists)
	order := make([]*Panelist, n)
	for i := 0; i < n; i++ {
		order[i] = c.Panelists[(c.startingOffset+i)%n]
	}
	return order
}

// RunIntroductions has each panelist briefly introduce themselves.
func (c *Coordinator) RunIntroductions(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	fmt.Println()
	fmt.Println("--- Panel Introductions ---")
	fmt.Println()

	for i, panelist := range c.Panelists {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Generate a brief introduction
		intro, err := panelist.GenerateResponse(ctx, c.Topic,
			"(Beginning of discussion)",
			fmt.Sprintf("Please introduce yourself briefly in one sentence. You are panelist %d of %d.", i+1, len(c.Panelists)))
		if err != nil {
			log.Printf("Error generating intro for %s: %v", panelist.Config.Name, err)
			continue
		}

		fmt.Printf("[%s]: \"%s\"\n", panelist.Config.Name, intro)
		c.Transcript.Add(panelist.Config.Name, intro)

		if err := panelist.Speak(ctx, intro); err != nil {
			log.Printf("Error speaking intro for %s: %v", panelist.Config.Name, err)
		}

		// Brief pause between introductions
		if i < len(c.Panelists)-1 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(1 * time.Second):
			}
		}
	}

	fmt.Println()
	fmt.Println("--- Introductions complete. Ready for questions! ---")
	fmt.Println()
	return nil
}

// GetTranscript returns the full formatted transcript.
func (c *Coordinator) GetTranscript() string {
	return c.Transcript.Format()
}
