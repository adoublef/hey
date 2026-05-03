mod error;

use crate::{cbz, eve, net::http::error::Error};
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
    cbz_client: cbz::Client,
}

pub fn app(eve_client: eve::Client, cbz_client: cbz::Client) -> Router {
    let state = AppState {
        eve_client,
        cbz_client,
    };
    Router::new()
        .merge(handle_orders())
        .merge(handle_weeb())
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

    route("/evetech/orders", get(handler))
}

fn handle_weeb() -> Router<AppState> {
    #[derive(Deserialize)]
    struct Params {
        series_url: Url,
        deflate: Option<bool>,
    }

    async fn handler(
        State(state): State<AppState>,
        Query(params): Query<Params>,
    ) -> Result<impl IntoResponse> {
        let stream = state.cbz_client.series(params.series_url);

        let response = Response::builder()
            .header(
                header::CONTENT_TYPE,
                mime::APPLICATION_OCTET_STREAM.essence_str(),
            )
            .header(
                header::CONTENT_DISPOSITION,
                "attachment; filename=\"weeb.zip\"",
            )
            .status(StatusCode::OK)
            .body(Body::from_stream(stream))?;

        Ok(response)
    }

    route("/cbz", get(handler))
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
