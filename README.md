# How to Run

1. Save your `sample_data.csv` to a Google Cloud Platform (GCP) bucket. I used `tmp-bucket-test`, but you can use any bucket of your choice. Just remember to update the bucket name constant in the code.
2. Create a service account with access to this bucket.
3. Download and save the `key.json` for this service account locally.
4. Run the dependencies using Docker:

   ```bash
   docker compose up -d
   ```
5. Set up the service account key by running:

   ```bash
   export GOOGLE_APPLICATION_CREDENTIALS=./key.json
   ```
6. Install deps `go mod tidy`
7. Run the golang file `go run main.go`
8. You should see something like `2024/10/04 22:48:47 Processed 1000 records`
9. Open the http://localhost:8123/play
10. Run 
```select project_id, toDate(date) as date, SUM(volume_usd) as volume
from transactions
group by project_id, date;
```
11. You should see the final result - volume aggregations y projects/dates


## Architecture

- The idea is simple: stream data into a data lake (ClickHouse) and perform post-aggregations.
- The pipeline consists of a Go program that downloads the CSV file, parses it, extracts minimal required data, calculates the USD volume for transactions, and produces the results to Kafka.
- ClickHouse consumes the data from Kafka and stores it in a queue table.
- A materialized view processes this data and saves it to the final data lake table.
- You can then run aggregation queries over this data for your use cases.

---

## Comments

- I used current prices for calculating volumes, as I do not have access to the Pro version of CoinGecko, to retrieve historical data
- There are several areas where optimization and refactoring could be done:
  - Splitting everything into separate modules.
  - Adding unit tests.
  - Introducing concurrency.
  - Refactoring methods that currently duplicate functionality.
  - Improving error handling.
  
- Additionally, applying best practices for Data Engineering such as:
  - Deduplication of entities.
  - Data type validations.
  - Consistency checks.
  - Implementing a schema registry (or similar tool).
  
- Many of these improvements were not applied due to time constraints. While we initially discussed a two-hour task, I ended up dedicating around 10 hours to this.
