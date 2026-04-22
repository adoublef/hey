# resources

## workspaces

```md
1. Run cargo add tokio --package some-crate to discover the latest version
1. Delete what it wrote
1. Manually add to [workspace.dependencies] in root Cargo.toml
1. Manually write tokio = { workspace = true } in each member crate
```

- [Workspaces, the dependencies table](https://doc.rust-lang.org/cargo/reference/workspaces.html#the-dependencies-table)
- [Using `derive_more` for errors in rust](https://quamserena.com/2025-08-02/using-derive-more-for-errors-in-rust)
