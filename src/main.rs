use clap::{Args, Parser, Subcommand};
use hey::{
    net::http::app,
    os::signal,
    runtime::{self},
};
use std::{env, ffi::OsString, process::ExitCode, time::Duration};
use tokio::{net::TcpListener, select};
use tokio_util::{sync::CancellationToken, time::FutureExt as _};

use crate::error::{Error, ParseError};

const DEFAULT_PORT: u16 = 3000;
const DEFAULT_SHUTDOWN_TIMEOUT: Duration = Duration::from_secs(60);

fn main() -> ExitCode {
    use Error::*;

    let args = env::args_os();
    let getenv = |key: &str| env::var_os(key);

    let (msg, code) = match run(args, getenv) {
        Ok(_) => (None, ExitCode::SUCCESS),
        Err(Parse(e)) => (Some(e.to_string()), ExitCode::from(2)),
        Err(Other(e)) => (Some(e), ExitCode::FAILURE),
    }; // parse() -> runner or

    if let Some(msg) = msg {
        eprintln!("{0}", msg);
    }
    code
}

fn run<I, T, F>(args: I, getenv: F) -> Result<(), Error>
where
    I: IntoIterator<Item = T>,
    T: Into<OsString> + Clone,
    F: Fn(&str) -> Option<OsString>, // result<T, ParseError>, where T impl FromStr
{
    use Command::*;

    match Cli::try_parse_from(args).map_err(ParseError::Flag)?.command {
        Serve(mut cfg) => {
            cfg.port = match getenv("PORT") {
                // `getenv` can handle mapping to ParseError::Utf8
                Some(port) if cfg.port == DEFAULT_PORT => port
                    .to_str()
                    .ok_or(ParseError::Utf8("PORT"))? // manually need to set this?
                    .parse()
                    .map_err(ParseError::Int)?,
                _ => cfg.port,
            };
            serve(cfg)?
        }
    }

    Ok(())
}

#[derive(Debug, Parser)]
struct Cli {
    #[clap(subcommand)]
    command: Command,
}

#[derive(Debug, Subcommand)]
enum Command {
    Serve(Serve),
}

#[derive(Debug, Args)]
struct Serve {
    #[clap(long, default_value_t = DEFAULT_PORT)]
    port: u16,
}

fn serve(cfg: Serve) -> Result<()> {
    runtime::build()
        // runtime error#1
        .map_err(|e| Error::Other(e.to_string()))?
        .block_on(async {
            // create listener
            let listener = TcpListener::bind(("0.0.0.0", cfg.port)).await?;
            let _addr = listener.local_addr()?;
            // todo: logger

            let token = CancellationToken::new();

            let server_token = token.child_token();
            let mut server_result = tokio::spawn(async move {
                axum::serve(listener, app())
                    .with_graceful_shutdown(async move { server_token.cancelled().await })
                    .await
            });

            select! {
                biased;
                Ok(res) = &mut server_result => match res {
                    Ok(()) => panic!("server shutdown prematurely"), // todo: return error instead
                    Err(error) => return Err(error)?,
                },
                res = signal::shutdown() => match res {
                    Ok(()) => token.cancel(),
                    Err(error) => return Err(error),
                },
            }

            match server_result.timeout(DEFAULT_SHUTDOWN_TIMEOUT).await {
                Ok(Ok(Ok(()))) => Ok(()),
                Ok(Ok(Err(error))) => Err(error)?,
                Ok(Err(_)) => unreachable!("we never cancel token"),
                Err(_) => Ok(()),
            }
        })
        // runtime error#2
        .map_err(|e| Error::Other(e.to_string()))
}

type Result<T, E = crate::error::Error> = core::result::Result<T, E>;

mod error {
    use derive_more::{Display, Error, From};

    #[derive(Debug, Display, Error, From)]
    pub enum Error {
        #[display("{_0}")]
        Parse(ParseError),
        #[display("other error: {_0}")]
        Other(#[error(not(source))] String),
    }

    // Clap args parse
    // ToStr parse
    // FromStr parse error
    // - [See more](https://github.com/JelteF/derive_more/issues/403)
    // non_exhuastive?
    #[derive(Debug, Display, Error, From)]
    pub enum ParseError {
        #[display("{_0}")]
        Flag(clap::Error),
        #[display("invalid UTF-8 in environment variable")]
        Utf8(#[error(not(source))] &'static str), // OsString
        #[display("invalid integer: {_0}")]
        Int(std::num::ParseIntError),
    }
}
