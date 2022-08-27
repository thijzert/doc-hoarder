Doc-hoarder is a document management system.

Born out of the necessity of archiving web pages for when the underlying site eventually disappears.


Building
--------
After installing dependencies using:
```
go get -v ./...
```
use the `build.go` script to compile the project. It has a few important options:

* `--base-url=https://x.y.z/f/b/`: Set this to the location at which the application will be hosted. (The compiled extension needs to know its own update URL in advance.)

A full example would be:

```
go run build.go --base-url=https://example.org/doc-hoarder/
```

### Signing the extension manually
In order to run the browser extension, it will need to be signed by Mozilla. Follow the instructions for self-distribution.
The XPI files you'll need to upload to Mozilla can be found in the directory `web/assets/extensions`. After getting approved, place the signed version in `web/assets/extensions/_signed`, and re-run the build script.

Image credits
-------------
* [Orangutan icons created by Flat Icons - Flaticon](https://www.flaticon.com/free-icons/orangutan)
