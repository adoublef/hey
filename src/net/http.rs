use axum::{
    Router,
    extract::State,
    response::{IntoResponse, Response},
    routing::get,
};
use http::StatusCode;
use sqlx::{Pool, Sqlite};

pub fn app(db: Pool<Sqlite>) -> Router {
    Router::new()
        .route("/", get(async || StatusCode::IM_A_TEAPOT))
        .route("/hey", get(async || "Hey, 👋🏿!"))
        .route("/ok", get(ok))
        .with_state(AppState(db))
}

async fn ok(State(st): State<AppState>) -> Result<()> {
    let _result = sqlx::query("SELECT 1").execute(&st.0).await?;

    Ok(())
}

#[derive(Debug, Clone)]
struct AppState(Pool<Sqlite>);

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
type Error = Box<dyn std::error::Error>;
