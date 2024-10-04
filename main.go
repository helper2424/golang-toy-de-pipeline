package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"encoding/csv"
	"encoding/json"

	"cloud.google.com/go/storage"
	"github.com/confluentinc/confluent-kafka-go/kafka"
	"github.com/shopspring/decimal"
)

const (
	TimeLayout  = "2006-01-02 15:04:05.000"
	BucketName  = "tmp-bucket-test"
	ObjectName  = "sample_data.csv"
	KafkaBroker = "localhost:29092"
)

var (
	Topic string = "transactions"
)

// Transaction
// --------------------------------------------------
type Transaction struct {
	Date       time.Time       `json:"date"`
	Project_id string          `json:"project_id"`
	Symbol     string          `json:"symbol"`
	Amount     decimal.Decimal `json:"amount"`
	VolumeUsd  decimal.Decimal `json:"volume_usd"`
}

func BuildTransaction(record []string) (*Transaction, error) {
	date, err := time.Parse(TimeLayout, record[1])

	if err != nil {
		return &Transaction{}, err
	}

	var numField NumField

	err = json.Unmarshal([]byte(record[14]), &numField)

	if err != nil {
		return &Transaction{}, err
	}

	var props PropsField

	err = json.Unmarshal([]byte(record[15]), &props)

	if err != nil {
		return &Transaction{}, err
	}

	return &Transaction{
		Date:       date,
		Project_id: record[0],
		Symbol:     numField.Pair,
		Amount:     props.Amount,
		VolumeUsd:  props.Amount,
	}, nil
}

// Prices
// --------------------------------------------------

type NumField struct {
	Pair string `json:"currencySymbol"`
}

type PropsField struct {
	Amount decimal.Decimal `json:"currencyValueDecimal"`
}

type Coin struct {
	ID     string `json:"id"`
	Symbol string `json:"symbol"`
	Name   string `json:"name"`
}

type SymToId map[string]string
type IdToSym map[string]string

// Price structure for the prices API response
type Price map[string]struct {
	USD float64 `json:"usd"`
}

type PriceService struct {
	prices map[string]decimal.Decimal
}

func GetCoins() *SymToId {
	mapping := make(SymToId)

	mapping["BTC"] = "bitcoin"
	mapping["ETH"] = "ethereum"
	mapping["BNB"] = "binancecoin"
	mapping["ADA"] = "cardano"
	mapping["SFL"] = "sunflower-land"
	mapping["USDC"] = "usd-coin"
	mapping["USDC.E"] = "usd-coin-ethereum-bridged"
	mapping["MATIC"] = "matic-network"

	// ...
	return &mapping
}

func ReverseCoinMapping() *IdToSym {
	symToIdMapping := GetCoins()

	mapping := make(IdToSym)

	for k, v := range *symToIdMapping {
		mapping[v] = k
	}

	return &mapping
}

func (ps PriceService) GetPrice(coin string) (decimal.Decimal, error) {
	if price, ok := ps.prices[coin]; ok {
		return price, nil
	}
	return decimal.NewFromInt(0), fmt.Errorf("price not found for coin %s", coin)
}

func (ps *PriceService) LoadPrices(symbols []string) error {
	coins := GetCoins()

	coindIds := make([]string, 0)

	for _, v := range symbols {
		if coin, ok := (*coins)[v]; ok {
			coindIds = append(coindIds, coin)
		}
	}

	prices, err := getPrices(coindIds)
	if err != nil {
		return fmt.Errorf("Failed to get prices: %v", err)
	}

	idToSymMapping := ReverseCoinMapping()

	for coin, price := range prices {
		ps.prices[(*idToSymMapping)[coin]] = decimal.NewFromFloat(price.USD)
	}

	return nil
}

func NewPriceService() *PriceService {
	return &PriceService{prices: make(map[string]decimal.Decimal)}
}

// Function to get the current prices for a list of coin IDs
func getPrices(ids []string) (Price, error) {
	client := &http.Client{Timeout: 10 * time.Second}

	// Convert the list of coin IDs into a comma-separated string
	idsString := ""
	for i, id := range ids {
		if i > 0 {
			idsString += ","
		}
		idsString += id
	}

	// Request the current prices from CoinGecko
	url := fmt.Sprintf("https://api.coingecko.com/api/v3/simple/price?ids=%s&vs_currencies=usd", idsString)
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var prices Price
	if err := json.NewDecoder(resp.Body).Decode(&prices); err != nil {
		return nil, err
	}

	var respBody []byte

	resp.Body.Read(respBody)

	return prices, nil
}

// --------------------------------------------------

func GetCsvReader(ctx context.Context, bucketName string, objectName string) (*csv.Reader, error) {
	client, err := storage.NewClient(ctx)
	if err != nil {
		return nil, err
	}
	defer client.Close()

	rc, err := client.Bucket(bucketName).Object(objectName).NewReader(ctx)
	if err != nil {
		return nil, err
	}

	defer rc.Close()

	return csv.NewReader(rc), nil
}

func main() {
	ctx := context.Background()
	reader, err := GetCsvReader(ctx, BucketName, ObjectName)

	if err != nil {
		log.Fatalf("failed to read csv: %v", err)
	}

	_, err = reader.Read() // skip header

	if err != nil {
		log.Fatalf("failed to read header: %v", err)
	}

	p, err := kafka.NewProducer(&kafka.ConfigMap{"bootstrap.servers": KafkaBroker})
	if err != nil {
		log.Fatalf("Failed to create producer: %v", err)
	}
	defer p.Close()

	transactions := make([]Transaction, 0)
	symbols := make(map[string]bool, 0)

	for {
		record, err := reader.Read()
		if err != nil {
			break
		}

		transaction, err := BuildTransaction(record)

		if err != nil {
			log.Fatalf("Failed to build transaction: %v", err)
			continue
		}

		transactions = append(transactions, *transaction)
		symbols[transaction.Symbol] = true
	}

	symbolsList := make([]string, 0)

	for symbol := range symbols {
		symbolsList = append(symbolsList, symbol)
	}

	priceService := NewPriceService()
	priceService.LoadPrices(symbolsList)

	for _, transaction := range transactions {
		price, err := priceService.GetPrice(transaction.Symbol)

		if err != nil {
			log.Fatalf("Failed to get price: %v", err)
			continue
		}

		transaction.VolumeUsd = price.Mul(transaction.Amount)

		marshaledTransaction, err := json.Marshal(transaction)

		if err != nil {
			log.Fatalf("Failed to marshal transaction: %v", err)
			continue
		}

		message := kafka.Message{
			TopicPartition: kafka.TopicPartition{Topic: &Topic, Partition: kafka.PartitionAny},
			Value:          marshaledTransaction,
		}

		// Produce the message to Kafka
		err = p.Produce(&message, nil)
		if err != nil {
			log.Printf("Failed to produce message: %v", err)
		}
	}

	p.Flush(5000)

	log.Printf("Processed %d records", len(transactions))
}
