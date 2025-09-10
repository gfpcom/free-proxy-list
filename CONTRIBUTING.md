# Contributing to Free Proxy List

First off, thank you for considering contributing to this project! Your help is greatly appreciated. This project is a community effort, and every contribution, no matter how small, is valuable.

This document provides guidelines for contributing to the project. Please read it carefully to ensure a smooth and effective contribution process.

## How Can I Contribute?

There are several ways you can contribute to this project:

- **Adding new proxy sources:** This is the easiest and most common way to contribute.
- **Adding new data transformers or parsers:** If you find a proxy source with a unique data format, you can add the logic to process it.
- **Improving the core application:** Enhancing performance, fixing bugs, or adding new features.
- **Reporting bugs or suggesting features:** If you find an issue or have an idea for a new feature, please open an issue.

### Other Ways to Contribute

Even if you don't have time to contribute code, there are other ways you can support the project:

- **Star our repository:** A star is a great way to show your appreciation and helps increase the project's visibility.
- **Share the repository:** Share the project with your friends, colleagues, or on social media. The more people who know about it, the better!

### Pull Request Process

1.  Ensure any install or build dependencies are removed before the end of the layer when doing a build.
2.  Update the `README.md` with details of changes to the interface, this includes new environment variables, exposed ports, useful file locations, and container parameters.
3.  Increase the version numbers in any examples and the `README.md` to the new version that this Pull Request would represent. The versioning scheme we use is [SemVer](http://semver.org/).
4.  You may merge the Pull Request in once you have the sign-off of two other developers, or if you do not have permission to do that, you may request the second reviewer to merge it for you.

## Getting Started: Your First Contribution

Unsure where to begin? A great way to start is by adding new proxy sources.

### Adding New Proxy Sources

The core of this project is the collection of proxy sources. We are always looking for new, reliable sources of free proxies.

**How it works:**

The application reads files from the `/sources` directory. Each file in this directory (e.g., `http.txt`, `vless.txt`) corresponds to a proxy protocol. The content of these files are URLs, with each URL pointing to a list of proxies.

**Steps to add a source:**

1.  **Find a proxy source URL.** This should be a raw text URL that provides a list of proxies.
2.  **Identify the correct file.** In the `/sources` directory, find the file that matches the protocol of your source list (e.g., for a list of HTTP proxies, use `http.txt`). If a file for that protocol doesn't exist, you can create one.
3.  **Add the URL.** Add the URL to a new line in the appropriate file.

**Advanced Source Configuration:**

Sometimes, a source provides data in a non-standard format. Our application uses **Transformers** and **Parsers** to handle these cases. You can specify them in the source file on the same line as the URL, separated by commas.

**URL Tokens:**

You can use dynamic tokens in the source URLs to fetch lists that are generated based on the current date and time. The application will replace these tokens with the current values.

-   `{YYYY}`: Full year (e.g., 2023)
-   `{MM}`: Zero-padded month (e.g., 09)
-   `{M}`: Month (e.g., 9)
-   `{DD}`: Zero-padded day (e.g., 05)
-   `{HH}`: Zero-padded hour (e.g., 08)
-   `{mm}`: Zero-padded minute (e.g., 01)
-   `{HH/N}`: Hour rounded to the nearest increment of N. For example, if it's 14:00, `{HH/6}` would resolve to `12`. This is useful for sources that update at regular intervals (e.g., every 6 hours).

**Format:** `url,transformer,parser`

-   **`url`**: (Required) The URL of the proxy list.
-   **`transformer`**: (Optional) Specifies how to transform the raw data before parsing. The default is `raw` (no transformation). We also have `base64` for sources encoded in Base64.
-   **`parser`**: (Optional) Specifies how to parse individual lines from the source. The default `ParseProxyURL` handles standard proxy URLs. Other options include `ColonURL` (for `ip:port` formats) and `SpaceURL` (for `ip port` formats).

**Example:**

Let's say you have a source for SOCKS5 proxies at `http://myproxies.com/list`. The list is Base64 encoded, and each proxy is in the `ip:port` format. You would add the following line to `sources/socks5.txt`:

```
http://myproxies.com/list,base64,ColonURL
```

### Adding a New Transformer

If a proxy source uses a unique encoding or format (e.g., Gzip, custom text format), you might need to add a new `Transformer`.

1.  **Go to `internal/transformer.go`**.
2.  Define a new function that matches the `Transformer` type: `func([]byte) []byte`. This function will take the raw response body and return the transformed body.
3.  **Register your new transformer** in the `init()` function within `internal/transformer.go`, giving it a name.

```go
// In internal/transformer.go
func init() {
    Transformers["base64"] = FromBase64
    Transformers["myNewTransformer"] = MyNewTransformer // Add your transformer here
}

// Define your transformer function
func MyNewTransformer(buf []byte) []byte {
    // ... your transformation logic ...
    return transformedBuf
}
```

### Adding a New Parser

If a source list has a unique structure for defining proxies that our existing parsers can't handle, you can add a new `Parser`.

1.  **Go to `internal/parser.go`**.
2.  Define a new function that matches the `Parser` type: `func(string, string) (*Proxy, error)`. This function takes the protocol and a line of text and should return a `Proxy` object or an error.
3.  **Register your new parser** in the `init()` function within `internal/parser.go`.

```go
// In internal/parser.go
func init() {
    Parsers["ColonURL"] = ParseColonURL
    Parsers["myNewParser"] = MyNewParser // Add your parser here
}

// Define your parser function
func MyNewParser(proto, line string) (*Proxy, error) {
    // ... your parsing logic ...
    return proxy, nil
}
```

## Code Style

This project uses standard Go formatting. Please run `go fmt` on your code before submitting a pull request. We also use a linter to ensure code quality.

Thank you for your contribution!
