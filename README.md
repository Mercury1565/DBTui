# DBTui

A lightweight, terminal-based user interface (TUI) for PostgreSQL built with Go.

`DBTui` provides a simple, keyboard-driven way to browse your PostgreSQL database schemas, tables, and columns, preview data, and run ad-hoc SQL queries.

## Features

- **Schema Navigation**: Easily browse through schemas, tables, and columns using a familiar tree-like interface.
- **Data Preview**: Select a table to automatically view its first N rows (configurable).
- **Ad-Hoc Queries**: Run custom SQL queries in a dedicated text area.
- **Keyboard Shortcuts**: Navigate the entire application using simple key presses.
  - `q`: Quit the application.
  - `r`: Refresh the list of schemas and tables.
  - `F5`: Execute the SQL query in the text area.
  - `Tab`: Cycle focus between the different panels (schemas, tables, columns, results, query).
- **Read-Only by Default**: The application is designed for safe exploration. Write operations are only performed when explicitly typed and executed in the query panel.

## Installation

### Prerequisites

- **Go**: Version 1.18 or higher.
- **PostgreSQL**: A PostgreSQL database to connect to.

### Using `go install`

The simplest way to install `DBTui` is by using the `go install` command. This will download the source, build the executable, and place it in your `$GOPATH/bin` directory. Ensure that `$GOPATH/bin` is in your system's `PATH`.

```bash
go install github.com/Mercury1565/DBTui@latest
```

## Usage

`DBTui` requires a PostgreSQL connection URL to run. You can provide this in one of two ways:

### 1. Using the `DATABASE_URL` Environment Variable (Recommended)

Set the `DATABASE_URL` in your shell before running the application.

```bash
export DATABASE_URL="postgres://user:pass@host:5432/dbname?sslmode=disable"
DBTui
```

### 2. Using the `-url` Flag

Provide the connection string directly via the command-line flag.

```bash
DBTui -url "postgres://user:pass@host:5432/dbname?sslmode=disable"
```

### Options

- `-url <connection_string>`: Specifies the PostgreSQL connection URL. This overrides the `DATABASE_URL` environment variable.
- `-limit <number>`: Sets the maximum number of rows to display in a table preview. The default is 100.

### Example

```bash
DBTui -url "postgres://user:pass@host:5432/dbname" -limit 200
```

## Contributing

Contributions are welcome! Please feel free to submit issues or pull requests to the [GitHub repository](https://github.com/Mercury1565/DBTui).

## License

This project is licensed under the MIT License. See the [LICENSE](LICENSE) file for details.
