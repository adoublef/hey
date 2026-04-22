use tokio::{select, signal};

pub async fn shutdown() -> Result<()> {
    select! {
        res = sigint() => res,
        res = sigterm() => res,
    }
}

async fn sigint() -> Result<()> {
    Ok(signal::ctrl_c().await?)
}

#[cfg(unix)]
async fn sigterm() -> Result<()> {
    use tokio::signal::unix;

    match unix::signal(unix::SignalKind::terminate()) {
        Ok(mut signal) => match signal.recv().await {
            Some(()) => Ok(()),
            None => panic!("could not listen for more SIGTERM events"), // return error
        },
        Err(error) => Err(error)?,
    }
}

#[cfg(not(unix))]
async fn sigterm() {
    std::future::pending().await
}

type Result<T, E = Error> = core::result::Result<T, E>;
type Error = Box<dyn std::error::Error>;
