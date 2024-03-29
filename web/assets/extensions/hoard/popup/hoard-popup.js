
const hidePage = `body { border: 2rem solid red !important; }`;

const logMessage = (msg, ...classes) => {
	let rv = document.createElement("div");

	for ( let c of classes ) {
		rv.classList.add(c);
	}

	if ( !(msg instanceof Array) && !(msg instanceof NodeList) ) {
		msg = [msg];
	}

	for ( let m of msg ) {
		if ( typeof(m) === "string" ) {
			let p = document.createElement("P");
			p.textContent = m;
			rv.appendChild(p);
		} else if ( typeof(m) === "object" && !!m.innerHTML ) {
			rv.appendChild(m);
		}
	}

	document.getElementById("error-log").appendChild(rv);

	window.setTimeout(() => {
		rv.style.opacity = 0;
	}, 7000);
	window.setTimeout(() => {
		if ( rv && rv.parentNode ) {
			rv.parentNode.removeChild(rv);
		}
	}, 7520);
};

/**
 * Listen for clicks on the buttons, and send the appropriate message to
 * the content script in the page.
 */
const listenForClicks = () => {
	const BASE_URL = "https://xxxxxxxxxxxxxxxxxxxxxxxx";

	let apikeyInput = document.querySelector("#settings #apikey") || document.createElement("input");
	let currentAPIKey = apikeyInput.value;
	let apikeyStatus = document.querySelector("#settings #apikey-status") || document.createElement("div");

	let saveApiKey = async (e) => {
		if ( apikeyInput.value == currentAPIKey ) {
			return;
		}
		currentAPIKey = apikeyInput.value;

		apikeyStatus.innerText = "…";

		try {
			let form = new FormData();
			form.append("api_key", apikeyInput.value);
			await browser.storage.sync.set({"hoard-api-key": apikeyInput.value});
			let resp = await fetch(BASE_URL + "api/user/whoami", {
				method: "POST",
				body: form
			});
			let who = await resp.json();
			if ( who.ok ) {
				apikeyStatus.innerText = `\u2713 Hi ${who.hello}`;
			} else {
				apikeyStatus.innerText = `\u274c`;
			}
		} catch ( e ) {
			apikeyStatus.innerText = "\u274c";
			console.error(e);
		}
	};
	apikeyInput.addEventListener("keyup", saveApiKey);
	apikeyInput.addEventListener("change", saveApiKey);

	document.addEventListener("click", async (e) => {

		const flatten = async (tabs) => {
			let txid_post = {method: "POST", body: new FormData()};
			txid_post.body.append("api_key", apikeyInput.value);
			txid_post.body.append("page_url", tabs[0].url);

			let txid = await fetch(BASE_URL + "api/capture-new-doc", txid_post);
			txid = await txid.json();
			if ( !txid.id ) {
				logMessage("OOK" + JSON.stringify(txid), "error");
				return;
			}

			let rv = await browser.tabs.sendMessage(tabs[0].id, {
				command: "flatten",
				doc_id: txid.id,
				txid: txid.txid,
			});

			if ( rv ) {
				let p = document.createElement("P");
				p.textContent = "ook: g" + rv.document_id;
				if ( rv.full_url ) {
					let a = document.createElement("A");
					a.textContent = "g" + rv.document_id;
					a.target = "_blank";
					a.href = rv.full_url;
					p.textContent = "ook: ";
					p.appendChild(a);
				}
				logMessage(p);
			} else {
				logMessage("OOK", "error");
			}
		}


		let actionName = "action";
		try {
			let currentTab = await browser.tabs.query({active: true, currentWindow: true})
			if ( !e.target.classList.contains("button") ) {
			} else if ( e.target.classList.contains("disabled") ) {
			} else if ( e.target.classList.contains("flatten") ) {
				actionName = "flatten"
				await flatten(currentTab);
			} else {
				logMessage("Unknown button class: " + e.target.getAttribute("class"), "error");
			}
		} catch ( e ) {
			logMessage(e.message, "error");
			console.error(`Error on ${actionName}`, e);
		}
	});
}

/**
 * There was an error executing the script.
 * Display the popup's error message, and hide the normal UI.
 */
function reportExecuteScriptError(error) {
	console.error(`Failed to execute content script: ${error.message}`);
	document.querySelector("#popup-content").classList.add("hidden");
	//document.querySelector("#error-content").classList.remove("hidden");
}


(async () => {
	let apikey = await browser.storage.sync.get("hoard-api-key");
	apikey = apikey["hoard-api-key"];
	if ( typeof apikey != "string" ) {
		apikey = "";
	}

	let input = document.querySelector("#settings #apikey");
	console.log("apikey ", apikey, " ; input ", input);
	if ( input ) {
		input.value = apikey;
	}
	if ( typeof apikey != "string" || apikey.length < 10 ) {
		let s = document.getElementById("settings");
		if ( s ) {
			s.open = true;
			if ( input ) {
				input.focus();
			}
		}
	}
})()


/**
 * When the popup loads, inject a content script into the active tab,
 * and add a click handler.
 * If we couldn't inject the script, handle the error.
 */
browser.tabs.executeScript({file: "/content_scripts/hoard.js"})
	.then(listenForClicks)
	.catch(reportExecuteScriptError);


console.log("hello, world");

