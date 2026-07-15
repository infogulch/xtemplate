# filebrowser example

Lists a directory and shows file contents using the `.FS` dot provider
(`.FS.ReadDir`, `.FS.Read`, `.FS.Exists`). With `"writable": true`, each
listing also offers multipart upload via `.FS.ReceiveFiles` (files land under
`{dir}/{request-id}/` with sequential names and an `upload.json` manifest).

```sh
mise run example-filebrowser
```

Then open http://localhost:9004/

```sh
mise run test-example-filebrowser
```
