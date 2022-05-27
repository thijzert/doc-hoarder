
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
	document.addEventListener("click", async (e) => {

		const flatten = async (tabs) => {
			let rv = await browser.tabs.sendMessage(tabs[0].id, {
				command: "flatten"
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
			if ( e.target.classList.contains("flatten") ) {
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
	document.querySelector("#popup-content").classList.add("hidden");
	document.querySelector("#error-content").classList.remove("hidden");
	console.error(`Failed to execute content script: ${error.message}`);
}

/**
 * When the popup loads, inject a content script into the active tab,
 * and add a click handler.
 * If we couldn't inject the script, handle the error.
 */
browser.tabs.executeScript({file: "/content_scripts/hoard.js"})
	.then(listenForClicks)
	.catch(reportExecuteScriptError);

console.log("hello, world");

