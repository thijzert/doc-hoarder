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
			console.log(uploadResult);
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

		let cssAttachments = {};
		let attachCSS = async (stylesheet) => {
			console.log("Attaching CSS ", stylesheet.href);
			if ( !stylesheet.href || cssAttachments.hasOwnProperty(stylesheet.href) ) {
				console.log("reattach limbs, not style sheets");
				return cssAttachments[stylesheet.href];
			}

			let att_id = {method: "POST", body: new FormData()}
			att_id.body.append("api_key", API_KEY);
			att_id.body.append("doc_id", doc_id.id);
			att_id.body.append("ext", "css");
			att_id = await fetch(BASE_URL + "api/new-attachment", att_id)
			att_id = await att_id.json();
			let filename = att_id.filename;
			att_id = att_id.attachment_id;
			console.log("filename: ", filename);
			cssAttachments[stylesheet.href] = filename;

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
			} catch ( _e ) { }

			await uploadFile(BASE_URL + "api/upload-attachment", "attachment", filename, attcnt, {
				"api_key": API_KEY,
				"doc_id": doc_id.id,
				"att_id": att_id,
			});

			return cssAttachments[stylesheet.href];
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
		console.log("Contents: ", contents.length, contents.substr(0,500));


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

