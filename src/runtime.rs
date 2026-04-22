use tokio::runtime::{Builder, Runtime};

pub fn build(/* */) -> Result<Runtime> {
    Ok(Builder::new_multi_thread().enable_all().build()?)
}

type Result<T, E = Error> = core::result::Result<T, E>;
type Error = Box<dyn std::error::Error>; //?
