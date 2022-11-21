
(async () => {
	const newKeyButton = document.getElementById("btn-new-api-key");
	const newKeyDialog = document.getElementById("dlg-new-api-key");

	newKeyButton.addEventListener("click", async () => {
		let label = `API key created at ${(new Date).toLocaleString()}`;

		let form = new FormData();
		form.set("label", label);
		let rq = await fetch("api/user/new-api-key", {
			method: "POST",
			body: form,
		});
		let data = await rq.json();
		console.log(data);

		newKeyDialog.querySelector("code.-apikey").textContent = data.apikey;
		newKeyDialog.showModal();
	});

	newKeyDialog.querySelector("button.-close").addEventListener("click", () => {
		newKeyDialog.close();
		location.reload();
	});
})()

