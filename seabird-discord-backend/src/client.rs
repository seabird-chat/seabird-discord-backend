use std::sync::Arc;

use async_trait::async_trait;
use envconfig::Envconfig;

use serenity::{
    client::EventHandler,
    model::channel::Message,
    model::{
        gateway::GatewayIntents,
        prelude::{Guild, Ready, UnavailableGuild},
    },
    prelude::Context,
};

#[derive(Envconfig)]
pub struct Config {
    #[envconfig(from = "DISCORD_TOKEN")]
    discord_token: String,

    #[envconfig(from = "DISCORD_COMMAND_PREFIX", default = "!")]
    command_prefix: String,

    #[envconfig(from = "SEABIRD_ID", default = "seabird")]
    seabird_id: String,

    #[envconfig(from = "SEABIRD_HOST")]
    seabird_host: String,

    #[envconfig(from = "SEABIRD_TOKEN")]
    seabird_token: String,
}

pub struct Client {
    command_prefix: String,
    seabird_client: seabird::ChatIngestClient,
    discord_client: serenity::Client,
}

impl Client {
    pub async fn new(config: Config) -> crate::Result<Self> {
        let seabird_config = seabird::ClientConfig {
            url: config.seabird_host,
            token: config.seabird_token,
        };
        let seabird_client = seabird::ChatIngestClient::new(seabird_config).await?;

        let discord_client = serenity::Client::builder(
            config.discord_token,
            GatewayIntents::non_privileged()
                | GatewayIntents::GUILD_MEMBERS
                | GatewayIntents::GUILD_PRESENCES
                | GatewayIntents::MESSAGE_CONTENT,
        )
        .await?;

        Ok(Client {
            command_prefix: config.command_prefix,
            seabird_client,
            discord_client,
        })
    }

    pub async fn run(&self) -> crate::Result<()> {
        tokio::try_join!(self.run_chat_ingest(), self.run_discord())?;
        Ok(())
    }

    async fn run_chat_ingest(&self) -> crate::Result<()> {
        unimplemented!()
    }

    async fn run_discord(&self) -> crate::Result<()> {
        unimplemented!()
    }
}

#[async_trait]
impl EventHandler for Client {
    async fn message(&self, ctx: Context, msg: Message) {
        // send message event
    }

    async fn guild_create(&self, ctx: Context, guild: Guild, is_new: bool) {
        for (channel_id, channel) in guild.channels {
            // send channel join events
        }
    }

    async fn guild_delete(
        &self,
        ctx: Context,
        unavailable: UnavailableGuild,
        maybe_guild: Option<Guild>,
    ) {
        if let Some(guild) = maybe_guild {
            for (channel_id, channel) in guild.channels {
                // send channel leave events
            }
        }
    }

    async fn ready(&self, _: Context, ready: Ready) {
        println!("{} is connected!", ready.user.name);
    }
}
