use envconfig::Envconfig;

mod client;
use client::{Client, Config};

type Result<T> = anyhow::Result<T>;

#[tokio::main]
async fn main() -> Result<()> {
    pretty_env_logger::init_timed();

    let config = Config::init_from_env()?;

    let client = Client::new(config).await?;

    client.run().await
}
