
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
		location.reload(); // TODO: just add one key to the list
	});

	document.querySelector("ul.api-keys").addEventListener("click", async (e) => {
		console.log(e)
		if ( !e.target || !e.target.classList.contains("-js-delete-api-key") ) {
			return
		}
		let key_id = e.target.dataset.keyId;
		console.log("delete", key_id);

		try {
			let form = new FormData();
			form.set("key_id", key_id);
			let rq = await fetch("api/user/disable-api-key", {
				method: "POST",
				body: form,
			});
			let data = await rq.json();
			location.reload(); // TODO: just delete this row
		} catch ( e ) {
			console.error(e);
		}
	});
})()

