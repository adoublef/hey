mod error;

use crate::{eve, net::http::error::Error};
use axum::{
    Router,
    body::Body,
    extract::{Query, State},
    response::{IntoResponse, Response},
    routing::{MethodRouter, get},
};
use http::{StatusCode, header};
use serde::Deserialize;
use url::Url;

#[derive(Debug, Clone)]
struct AppState {
    eve_client: eve::Client,
}

pub fn app(order_handler: eve::Client) -> Router {
    let state = AppState {
        eve_client: order_handler,
    };
    Router::new()
        .merge(handle_orders())
        //...
        .with_state(state)
}

fn handle_orders() -> Router<AppState> {
    #[derive(Deserialize)]
    struct Params {
        base_url: Url,
    }

    async fn handler(
        State(state): State<AppState>,
        Query(params): Query<Params>,
    ) -> Result<impl IntoResponse> {
        let stream = state.eve_client.orders(params.base_url);

        let response = Response::builder()
            .header(header::CONTENT_TYPE, mime::TEXT_CSV.essence_str())
            .header(
                header::CONTENT_DISPOSITION,
                "attachment; filename=\"evetech.csv\"",
            )
            .status(StatusCode::OK)
            .body(Body::from_stream(stream))?;

        Ok(response)
    }

    route("/", get(handler))
}

fn route<T>(path: &str, method_router: MethodRouter<T>) -> Router<T>
where
    T: Clone + Send + Sync + 'static,
{
    Router::<T>::new().route(path, method_router)
}

struct AppError(Error);

impl<E> From<E> for AppError
where
    E: Into<Error>,
{
    fn from(err: E) -> Self {
        Self(err.into())
    }
}

impl IntoResponse for AppError {
    fn into_response(self) -> Response {
        (
            StatusCode::INTERNAL_SERVER_ERROR,
            format!("Something went wrong: {}", self.0),
        )
            .into_response()
    }
}

type Result<T, E = AppError> = core::result::Result<T, E>;
