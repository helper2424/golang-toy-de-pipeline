CREATE TABLE IF NOT EXISTS transactions
(
    `project_id` String  NOT NULL,
    `date` DateTime64(3)  NOT NULL,
    `symbol` String  NOT NULL,
    `amount` Decimal(76, 38),
    `volume_usd` Decimal(76, 38)
)
ENGINE = MergeTree()
ORDER BY (project_id)
SETTINGS index_granularity = 8192;

CREATE TABLE IF NOT EXISTS transactions_queue
(
    `project_id` String NOT NULL,
    `date` String NOT NULL,
    `symbol` String NOT NULL,
    `amount` String NOT NULL,
    `volume_usd` String NOT NULL
)
ENGINE = Kafka
SETTINGS kafka_broker_list = 'kafka:9092', kafka_topic_list = 'transactions', kafka_group_name = 'clickhouse', kafka_format = 'JSONEachRow', kafka_max_block_size = 1048576, kafka_num_consumers = 1, kafka_handle_error_mode = 'stream';

CREATE MATERIALIZED VIEW IF NOT EXISTS transactions_queue_mv TO transactions
(
    `project_id` String NOT NULL,
    `date` DateTime64(3) NOT NULL,
    `symbol` String NOT NULL,
    `amount` Decimal(76, 38),
    `volume_usd` Decimal(76, 38)
) AS
SELECT
    project_id,
    parseDateTimeBestEffort(date) as date,
    symbol,
    CAST(amount AS Decimal(76, 38)) as amount,
    CAST(volume_usd AS Decimal(76, 38)) as volume_usd
FROM transactions_queue;

CREATE MATERIALIZED VIEW IF NOT EXISTS transactions_queue_errors
(
    `topic` String NOT NULL,
    `partition` UInt64 NOT NULL,
    `offset` UInt64 NOT NULL,
    `raw` String,
    `error` String
)
ENGINE = MergeTree()
ORDER BY (topic, partition, offset)
SETTINGS index_granularity = 8192 AS
SELECT
    _topic AS topic,
    _partition AS partition,
    _offset AS offset,
    _raw_message AS raw,
    _error AS error
FROM bot_events_queue
WHERE length(_error) > 0;