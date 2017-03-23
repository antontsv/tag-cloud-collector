/*
 Package provides simple interface to collect
 popular items (words, sentenses, ideas) and ask users to rank them
 from the most interesting to the least interesting
*/
package main

import (
	"bufio"
	"context"
	"crypto/sha256"
	"math/rand"
	"os"
	"os/user"
	"strconv"
	"strings"
	"time"

	elastic "gopkg.in/olivere/elastic.v5"

	"fmt"
)

// ElasticSearch index name to store the entries
const targetIndex = "populatity"

// ElasticSearch type name to be used under selected index
const targetDoc = "votes"
const maxTopicsToQuery = 50

func exit(message string) {
	fmt.Println("We have a proble:")
	fmt.Println(message)
	os.Exit(1)
}

type TalkVote struct {
	User   string `json:"user"`
	Title  string `json:"title"`
	Points int    `json:"interestPoints"`
}

type TermAggregation struct {
	elastic.Aggregation
	name  string
	value interface{}
}

func (q *TermAggregation) Source() (interface{}, error) {
	source := make(map[string]interface{})
	tq := make(map[string]interface{})
	source["terms"] = tq
	tq[q.name] = q.value
	tq["size"] = maxTopicsToQuery
	return source, nil
}

func NewTermAggregation(name string, value interface{}) *TermAggregation {
	return &TermAggregation{name: name, value: value}
}

func AddVote(client *elastic.Client, topic string, points int) {
	usr, err := user.Current()
	if err != nil {
		exit("Sorry, cannot detect your username. Bye!")
	}
	vote := TalkVote{User: usr.Username, Title: topic, Points: points}
	h := sha256.New()
	h.Write([]byte(topic + usr.Username))
	_, err = client.Index().
		Index(targetIndex).
		Type(targetDoc).
		Id(fmt.Sprintf("%x", h.Sum(nil)[0:5])).
		BodyJson(vote).
		Do(context.Background())
	if err != nil {
		exit("Was unable to add new topic")
	}
}

func remove(s []string, i int) []string {
	s[len(s)-1], s[i] = s[i], s[len(s)-1]
	return s[:len(s)-1]
}

func main() {

	ctx := context.Background()
	// for trace loggin use: elastic.SetTraceLog(log.New(os.Stdout, "", 0))
	client, err := elastic.NewClient(
		elastic.SetURL("http://10.100.100.101:9200"),
		elastic.SetSniff(false))
	if err != nil {
		exit("No connection to Elastic search")
	}
	exists, err := client.IndexExists(targetIndex).Do(ctx)
	if err != nil {
		exit("Cannot query Elastic search")
	}

	if !exists {
		_, err = client.CreateIndex(targetIndex).Do(ctx)
		if err != nil {
			exit("Cannot create index in Elastic search")
		}
	}

	aggName := "topics"
	termAggregation := NewTermAggregation("field", "title.keyword")
	searchResult, err := client.Search().
		Index(targetIndex).
		Type(targetDoc).
		Aggregation(aggName, termAggregation).
		Size(0).
		Pretty(true).
		Do(ctx)
	if err != nil {
		exit("Unable to get list of existing topics")
	}

	aggResult, ok := searchResult.Aggregations.Terms(aggName)
	if ok && len(aggResult.Buckets) > 0 {
		fmt.Println("Exiting suggestions:")
		for i, item := range aggResult.Buckets {
			fmt.Printf("#%02d %s\n", i+1, item.Key)
		}
	} else {
		fmt.Println("No topics available")
	}
	fmt.Println()

	reader := bufio.NewReader(os.Stdin)

	for {
		fmt.Println("Do you have another topic in mind [y/N]?")
		fmt.Print(">> ")
		text, _ := reader.ReadString('\n')
		if strings.HasPrefix(strings.ToLower(text), "y") {
			fmt.Println("Ok, type it now:")
			fmt.Print(">> ")
			text, _ = reader.ReadString('\n')
			topic := strings.TrimSpace(text)
			if len(topic) > 0 {
				fmt.Println("New topic: ", topic)
				fmt.Println("Looks good [y/N]?")
				fmt.Print(">> ")
				text, _ = reader.ReadString('\n')
				if strings.HasPrefix(strings.ToLower(text), "y") {
					fmt.Println("Ok, I will create it")
					AddVote(client, topic, 5)
				} else {
					fmt.Println("Poof! Erased. We will pretend that you have never suggested it :)")
				}
			}
		} else {
			break
		}
	}

	fmt.Println("Ready to rank the topics [Y/n]?")
	fmt.Print(">> ")
	text, _ := reader.ReadString('\n')
	if !strings.HasPrefix(strings.ToLower(text), "n") {
		termAggregation = NewTermAggregation("field", "title.keyword")
		searchResult, err = client.Search().
			Index(targetIndex).
			Type(targetDoc).
			Aggregation(aggName, termAggregation).
			Size(0).
			Pretty(true).
			Do(ctx)
		if err != nil {
			exit("Unable to get list of existing topics")
		}

		aggResult, ok := searchResult.Aggregations.Terms(aggName)
		if ok && len(aggResult.Buckets) > 0 {
			fmt.Println("Starting to rank topics (to your liking, of course)")
			count := len(aggResult.Buckets)
			leftToRank := make([]string, count)
			rand.Seed(time.Now().UTC().UnixNano())
			perm := rand.Perm(count)
			for i, v := range perm {
				leftToRank[i] = aggResult.Buckets[v].Key.(string)
			}
			for i := 0; i < count; {
				fmt.Printf("Lets determine your #%d pick:\n", i+1)
				for j := 0; j < len(leftToRank); j++ {
					fmt.Printf("#%02d %s\n", j+1, leftToRank[j])
				}
				fmt.Print("[enter a number] >> ")
				text, _ := reader.ReadString('\n')
				selectedNumber, err := strconv.Atoi(strings.TrimSpace(text))
				if err != nil || selectedNumber < 1 || selectedNumber > len(leftToRank) {
					fmt.Println("There is no item with that number!")
					fmt.Println("Lets try again")
					continue
				} else {
					fmt.Println()
					fmt.Printf("Your #%d pick is: %s, [Y/n]\n", i+1, leftToRank[selectedNumber-1])
					fmt.Print(">> ")
					text, _ := reader.ReadString('\n')
					if strings.HasPrefix(strings.ToLower(text), "n") {
						fmt.Println("Ok, lets choose another one")
						continue
					}
					AddVote(client, leftToRank[selectedNumber-1], count-i)
					leftToRank = remove(leftToRank, selectedNumber-1)
				}
				i++
				if len(leftToRank) < 2 {
					AddVote(client, leftToRank[0], count-i-1)
					fmt.Println("Thanks for ranking all of the topics. The results are in!")
					break
				}
			}
		} else {
			fmt.Println("No topics available")
		}
		fmt.Println()
	} else {
		fmt.Println("Ok, but please come back to do so. Your opinion matters!")
	}

}
