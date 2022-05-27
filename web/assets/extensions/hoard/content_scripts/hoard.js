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
		let rv = "<!DOCTYPE " + doctype.name;
		if ( doctype.publicId != "" ) {
			rv += " PUBLIC \"" + doctype.publicId + "\"";
		}
		if ( doctype.systemId != "" ) {
			rv += "\"" + doctype.systemId + "\"";
		}
		return rv + ">";
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

		let p = doc.createElement("P");
		p.textContent = "Hello, world";
		doc.body.insertBefore(p, doc.body.firstChild);
		
		for ( let sc of doc.querySelectorAll("script") ) {
			rm(sc);
		}

		for ( let link of doc.querySelectorAll("link") ) {
			if ( link.rel != "stylesheet" ) {
				continue;
			}
			console.log("link", link.href, link.rel)

			let attcnt = await fetch(link.href);
			attcnt = await attcnt.blob();

			let att_id = {method: "POST", body: new FormData()}
			att_id.body.append("api_key", API_KEY);
			att_id.body.append("doc_id", doc_id.id);
			att_id.body.append("ext", "css");
			att_id = await fetch(BASE_URL + "api/new-attachment", att_id)
			att_id = await att_id.json();
			att_id = att_id.attachment_id;

			let uploadForm = new FormData();
			uploadForm.append("api_key", API_KEY);
			uploadForm.append("doc_id", doc_id.id);
			uploadForm.append("att_id", att_id);

			uploadForm.append("attachment", new File([attcnt], "a" + att_id + ".css"));

			let upload = await fetch(BASE_URL + "api/upload-attachment", {
				method: "POST",
				body: uploadForm,
			});
			let uploadResult = await upload.json();
			console.log(uploadResult);

			link.href = `att/a${att_id}.css`;
		}

		contents = formatDTD(document.doctype) + "\n" + doc.body.parentNode.outerHTML;
		console.log("Contents: ", contents.length, contents.substr(0,500));

		const CHUNK_SIZE = 512000;
		for ( let idx = 0; idx < contents.length; idx += CHUNK_SIZE ) {
			let uploadForm = new FormData();
			uploadForm.append("api_key", API_KEY);
			uploadForm.append("doc_id", doc_id.id);

			uploadForm.append("document", new File([new Blob([contents.substr(idx, CHUNK_SIZE)])], document.title + ".html"));

			let upload = await fetch(BASE_URL + "api/upload-draft", {
				method: "POST",
				body: uploadForm,
			});
			let uploadResult = await upload.json();
			console.log(uploadResult);
		}

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

