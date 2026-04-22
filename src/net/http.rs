use axum::{Router, routing::get};
use http::StatusCode;

pub fn app() -> Router {
    Router::new()
        .route("/", get(async || StatusCode::IM_A_TEAPOT))
        .route("/hey", get(async || "Hey, 👋🏿!"))
}
