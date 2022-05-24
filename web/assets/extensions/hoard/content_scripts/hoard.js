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



	const flatten = async () => {
		const BASE_URL = "https://xxxxxxxxxxxxxxxxxxxxxxxx";
		const API_KEY = "yyyyyyyyyyyyyyyyyyyyyyyyyyyyyyyy";

		let with_apikey = {method: "POST", body: new FormData()}
		with_apikey.body.append("api_key", API_KEY);

		let doc_id = await fetch(BASE_URL + "api/new-doc", with_apikey)
		doc_id = await doc_id.json();

		console.log("doc_id:", doc_id.id);


		let contents = document.doctype + "\n" + document.body.parentNode.outerHTML;

		let uploadForm = new FormData();
		uploadForm.append("api_key", API_KEY);
		uploadForm.append("doc_id", doc_id.id);

		uploadForm.append("document", new File([new Blob([contents])], document.title + ".html"));

		let upload = await fetch(BASE_URL + "api/upload-draft", {
			method: "POST",
			body: uploadForm,
		});
		let uploadResult = await upload.json();
		console.log(uploadResult);
	}


	/**
	 * Given a URL to a beast image, remove all existing beasts, then
	 * create and style an IMG node pointing to
	 * that image, then insert the node into the document.
	 */
	function insertBeast(beastURL) {
		// removeExistingBeasts();
		// let beastImage = document.createElement("img");
		// beastImage.setAttribute("src", beastURL);
		// beastImage.style.height = "100vh";
		// beastImage.className = "beastify-image";
		// document.body.appendChild(beastImage);
	}

	/**
	 * Listen for messages from the background script.
	 * Call "insertBeast()" or "removeExistingBeasts()".
	 */
	browser.runtime.onMessage.addListener((message) => {
		console.log("Got message", message);
		if (message.command === "beastify") {
			insertBeast(message.beastURL);
		} else if (message.command === "flatten") {
			flatten();
		}
	});

})();

