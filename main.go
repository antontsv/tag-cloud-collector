package main

import (
	"bufio"
	"context"
	"os"
	"os/user"
	"strings"

	elastic "gopkg.in/olivere/elastic.v5"

	"fmt"
)

const targetIndex = "brownbag"
const targetDoc = "talks"

func exit(message string) {
	fmt.Println("We have a problem:")
	fmt.Println(message)
	os.Exit(1)
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
	return source, nil
}

func NewTermAggregation(name string, value interface{}) *TermAggregation {
	return &TermAggregation{name: name, value: value}
}

func main() {

	usr, err := user.Current()
	if err != nil {
		exit("Sorry, cannot detect your username. Bye!")
	}

	ctx := context.Background()
	client, err := elastic.NewClient(
		elastic.SetURL("http://10.100.100.101:9201"),
		elastic.SetSniff(false))
	if err != nil {
		exit("No connection to Elastic search")
	}
	_, err = client.IndexExists(targetIndex).Do(ctx)
	if err != nil {
		exit("Cannot query Elastic search")
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
		// Handle error
		panic(err)
	}

	aggResult, ok := searchResult.Aggregations.Terms(aggName)
	if ok && len(aggResult.Buckets) > 0 {
		fmt.Println("Exiting suggestions:")
		for i, item := range aggResult.Buckets {
			fmt.Printf("#%02d %s\n", i+1, item.Key)
		}
		fmt.Println("")
	}

	fmt.Println(usr.Username)
	fmt.Println("")

	reader := bufio.NewReader(os.Stdin)

	for {
		fmt.Println("Do you have another ntopic in mind [Y/N]?")
		fmt.Print(">> ")
		text, _ := reader.ReadString('\n')
		if strings.HasPrefix(strings.ToLower(text), "y") {
			fmt.Println("Ok, type it now:")
			fmt.Print(">> ")
			text, _ = reader.ReadString('\n')
			text = strings.TrimSpace(text)
			if len(text) > 0 {
				fmt.Println("New topic: ", text)
				fmt.Println("Looks good [Y/N]?")
				fmt.Print(">> ")
				text, _ = reader.ReadString('\n')
				if strings.HasPrefix(strings.ToLower(text), "y") {
					fmt.Println("Ok, I will create it")
				} else {
					fmt.Println("Poof! Erased. We will pretent that you have never suggested it :)")
				}
			}
		} else {
			break
		}
	}

}
