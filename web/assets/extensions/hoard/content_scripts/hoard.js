(function() {
	/**
	 * Check and set a global guard variable.
	 * If this content script is injected into the same page again,
	 * it will do nothing next time.
	 */
	if (window.hasRun) {
		return;
	}
	window.hasRun = true;


	const rm = (node) => {
		if ( node && node.forEach ) {
			node.forEach(rm);
			return;
		}
		if ( !node || !node.parentNode ) {
			return;
		}
		node.parentNode.removeChild(node);
	};


	// TODO: move to plugin-like architecture
	const siteHooks = [];
	siteHooks.push({re: new RegExp("^https://developer.mozilla.org/[^/]+/docs/.*$"), fn: (doc) => {
		rm(doc.querySelectorAll(".top-navigation-main"));
		rm(doc.querySelectorAll(".article-actions"));
		rm(doc.querySelectorAll(".mdn-cta-container"));
	}});


	const parser = new DOMParser();


	const formatDTD = (doctype) => {
		if (doctype === null) {
			return "<!DOCTYPE html>";
		}

		let rv = "<!DOCTYPE " + doctype.name;
		if ( doctype.publicId != "" ) {
			rv += " PUBLIC \"" + doctype.publicId + "\"";
		}
		if ( doctype.systemId != "" ) {
			rv += "\"" + doctype.systemId + "\"";
		}
		return rv + ">";
	}

	const UPLOAD_CHUNK_SIZE = 512000;
	const uploadFile = async (url, paramName, filename, contents, formParameters) => {
		if ( !(contents instanceof Blob) ) {
			contents = new Blob([contents]);
		}

		let idx = 0;
		do {
			let uploadForm = new FormData();
			if ( idx === 0 ) {
				uploadForm.append("truncate", "1")
			}

			for ( let k in formParameters ) {
				if ( formParameters.hasOwnProperty(k) ) {
					uploadForm.append(k, formParameters[k]);
				}
			}
			uploadForm.append(paramName, new File([contents.slice(idx, idx+UPLOAD_CHUNK_SIZE)], document.title + ".html"));

			let upload = await fetch(url, {
				method: "POST",
				body: uploadForm,
			});
			let uploadResult = await upload.json();
			idx += UPLOAD_CHUNK_SIZE;
		} while ( idx < contents.size );
	}

	const flatten = async () => {
		const BASE_URL = "https://xxxxxxxxxxxxxxxxxxxxxxxx";
		const API_KEY = "yyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyy";

		let with_apikey = {method: "POST", body: new FormData()}
		with_apikey.body.append("api_key", API_KEY);

		let doc_id = await fetch(BASE_URL + "api/new-doc", with_apikey)
		doc_id = await doc_id.json();

		console.log("doc_id:", doc_id.id);


		let contents = formatDTD(document.doctype) + "\n" + document.body.parentNode.outerHTML;
		let doc = parser.parseFromString(contents, "text/html");

		for ( let h of siteHooks ) {
			if ( h.re.test(location.href) ) {
				h.fn(doc);
			}
		}

		for ( let sc of doc.querySelectorAll("script") ) {
			rm(sc);
		}

		let imageAttachments = {};
		let attachImg = async (image_url) => {
			if ( imageAttachments.hasOwnProperty(image_url) ) {
				return imageAttachments[image_url];
			}
			console.log("getting image url ", image_url);

			let att_id = {method: "POST", body: new FormData()}
			att_id.body.append("api_key", API_KEY);
			att_id.body.append("doc_id", doc_id.id);

			let url = new URL(image_url, location.href);
			if ( url.origin !== location.origin ) {
				att_id.body.append("url", url.toString());
				att_id = await fetch(BASE_URL + "api/proxy-attachment", att_id)
				att_id = await att_id.json();
				imageAttachments[image_url] = att_id.filename;
				return att_id.filename;
			}

			let blob = await fetch(url);
			blob = await blob.blob();

			if ( blob.type.substr(0,6) != "image/" ) {
				throw "unknown mime type '" + blob.type + "'";
			}

			att_id.body.append("ext", blob.type.substr(6));
			att_id = await fetch(BASE_URL + "api/new-attachment", att_id)
			att_id = await att_id.json();
			let filename = att_id.filename;
			att_id = att_id.attachment_id;
			imageAttachments[image_url] = filename;

			await uploadFile(BASE_URL + "api/upload-attachment", "attachment", filename, blob, {
				"api_key": API_KEY,
				"doc_id": doc_id.id,
				"att_id": att_id,
			});

			return imageAttachments[image_url];
		};
		let tryAttach = async (u) => {
			if ( u != "" && u.substr(0,5) != "data:" ) {
				try {
					let s = await attachImg(u)
				} catch ( _e ) { console.error(_e); }
			}
		};


		for ( let img of document.images ) {
			await tryAttach(img.src);
			for ( let srcs of img.srcset.split(",") ) {
				srcs = srcs.trim().split(" ");
				await tryAttach(srcs[0]);
			}
		}
		for ( let img of doc.images ) {
			if ( imageAttachments.hasOwnProperty(img.src) ) {
				img.src = imageAttachments[img.src];
			}
			let srcset = [];
			for ( let srcs of img.srcset.split(",") ) {
				srcs = srcs.trim().split(" ");
				if ( imageAttachments.hasOwnProperty(srcs[0]) ) {
					srcs[0] = imageAttachments[srcs[0]];
				}
				srcset.push(srcs.join(" "));
			}
			img.srcset = srcset.join(", ");
		}

		let cssAttachments = {};
		let attachCSS = async (stylesheet) => {
			const stylesheet_href = stylesheet.href;
			if ( !stylesheet_href || cssAttachments.hasOwnProperty(stylesheet_href) ) {
				return cssAttachments[stylesheet_href];
			}
			console.log("Attaching CSS ", stylesheet_href);

			let att_id = null, filename = null;

			let url = new URL(stylesheet_href, location.href);
			if ( url.origin !== location.origin ) {
				att_id = {method: "POST", body: new FormData()}
				att_id.body.append("api_key", API_KEY);
				att_id.body.append("doc_id", doc_id.id);
				att_id.body.append("url", stylesheet_href);
				att_id = await fetch(BASE_URL + "api/proxy-attachment", att_id)
				att_id = await att_id.json();
				filename = att_id.filename;
				att_id = att_id.attachment_id;
				cssAttachments[stylesheet_href] = filename;

				style_cnt = {method: "POST", body: new FormData()}
				style_cnt.body.append("api_key", API_KEY);
				style_cnt.body.append("doc_id", doc_id.id);
				style_cnt.body.append("att_id", att_id);
				style_cnt = await fetch(BASE_URL + "api/download-attachment", style_cnt)
				style_cnt = await style_cnt.blob();
				style_cnt = await style_cnt.text();
				stylesheet = new CSSStyleSheet();
				stylesheet.replaceSync(style_cnt);
			} else {
				att_id = {method: "POST", body: new FormData()}
				att_id.body.append("api_key", API_KEY);
				att_id.body.append("doc_id", doc_id.id);
				att_id.body.append("ext", "css");
				att_id = await fetch(BASE_URL + "api/new-attachment", att_id)
				att_id = await att_id.json();
				filename = att_id.filename;
				att_id = att_id.attachment_id;
				cssAttachments[stylesheet_href] = filename;
			}


			let attcnt = "";

			try {
				for ( let rule of stylesheet.cssRules ) {
					if ( rule instanceof CSSImportRule ) {
						let imp = await attachCSS(rule.styleSheet);
						attcnt += `@import url("${imp}");`;
					} else {
						attcnt += rule.cssText;
					}
				}
			} catch ( _e ) { console.error(_e); }

			await uploadFile(BASE_URL + "api/upload-attachment", "attachment", filename, attcnt, {
				"api_key": API_KEY,
				"doc_id": doc_id.id,
				"att_id": att_id,
			});

			return cssAttachments[stylesheet_href];
		}

		for ( let stylesheet of document.styleSheets ) {
			await attachCSS(stylesheet);
		}

		for ( let link of doc.querySelectorAll("link") ) {
			if ( link.rel != "stylesheet" ) {
				continue;
			}

			let href = link.href.toString();
			if ( cssAttachments.hasOwnProperty(href) ) {
				link.href = cssAttachments[href];
			} else {
				link.rel = "defunct-stylesheet";
				console.error("css link not found", link.href, cssAttachments);
			}
		}

		contents = formatDTD(document.doctype) + "\n" + doc.body.parentNode.outerHTML;


		await uploadFile(BASE_URL + "api/upload-draft", "document", document.title + ".html", contents, {
			"api_key": API_KEY,
			"doc_id": doc_id.id,
		});

		return {
			success: true,
			document_id: doc_id.id,
			full_url: (new URL("documents/view/g" + doc_id.id + "/", BASE_URL)).toString(),
		};
	}


	// Listen for messages from the background script.
	browser.runtime.onMessage.addListener(async (message) => {
		console.log("Got message", message);
		if (message.command === "flatten") {
			return await flatten();
		}
	});

})();

