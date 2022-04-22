(async () => {
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
})()
