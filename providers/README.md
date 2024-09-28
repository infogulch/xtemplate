# xtemplate Dot Providers

Dot Providers is how xtemplate users can customize the template dot value
`{{.}}` to access dynamic functionality from within your templates.

This directory contains optional Dot Provider implementations created for and
maintained with xtemplate.

> [!NOTE]
>
> Users can implement and add their own dot providers by implementing the
> `xtemplate.DotProvider` interface and configuring xtemplate to use it.

> [!NOTE]
>
> xtemplate also exposes the `Config.FuncMaps`

## Providers

### `DotKV`

Add simple key-value string pairs to your templates. Could be used for runtime
config options for your templates.

### `DotDB`

Connect to a database with any available go driver by its name to run queries
and execute procedures against your database from within templates.

To use a driver, import its Go package while building your application.

### `DotFS`

Open a directory to list and read files with templates.
