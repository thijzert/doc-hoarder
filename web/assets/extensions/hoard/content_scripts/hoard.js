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
		if ( node && (node instanceof Array || node instanceof NodeList) ) {
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
	siteHooks.push({re: new RegExp("^https://(www\.)?ah\.nl/(allerhande/recept|recepten)/.*$"), fn: (doc) => {
		for ( let hroot of doc.querySelectorAll("#navigation-header > *[class^=header_root_]") ) {
			hroot.style.position = "static";
			hroot.style.tranform = "none";
		}
		rm(doc.querySelectorAll("#navigation-header > *[class^=header_root_] > *[class^=top-bar_root_"));
		rm(doc.querySelectorAll("#navigation-header > *[class^=header_placeholder_]"));
		rm(doc.querySelectorAll("button[class^=back-to-top_]"));
		rm(doc.querySelectorAll("button[class^=play-button_root_]"));
	}});



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

	const splitFirst = (s, sep) => {
		let a = s.indexOf(sep);
		if ( a < 0 ) {
			return [ s, "" ];
		}
		return [ s.slice(0,a), s.slice(a+sep.length) ];
	}
	const parseSrcset = (img) => {
		rv = [];
		let srcset = img.srcset.trim()
		while ( srcset != "" ) {
			let pts = splitFirst(srcset, " ");
			if ( pts[1] == "" ) {
				break
			}
			let url = pts[0];
			pts = splitFirst(pts[1].trim(), ",")
			let spec = pts[0];

			srcset = pts[1].trim();
			rv.push({url, spec});
		}

		return rv;
	};

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
			if ( !uploadResult.ok ) {
				console.error(uploadResult);
			}
			idx += UPLOAD_CHUNK_SIZE;
		} while ( idx < contents.size );
	}

	const flatten = async (txid, doc_id) => {
		const BASE_URL = "https://xxxxxxxxxxxxxxxxxxxxxxxx";

		const postDoc = async (addr, data, as = "json") => {
			let req = {method: "POST", body: new FormData()}
			req.body.append("txid", txid);
			for ( let k in data ) {
				if ( data.hasOwnProperty(k) ) {
					req.body.append(k, data[k]);
				}
			}

			let resp = await fetch(BASE_URL + addr, req)
			if ( !resp.ok ) {
				let err = await resp.json();
				throw err;
			}

			if ( as === "json" ) {
				return await resp.json();
			} else if ( as === "blob" ) {
				return await resp.blob();
			} else if ( as === "text" ) {
				// FIXME: maybe await resp.blob() and then await blob.text()?
				return await resp.text();
			} else {
				console.error(`Invalid return type ${as}; returning text`);
				return await resp.text();
			}
		};


		// Add an implicit ID to inline stylesheets - we'll need it later
		for ( let ss of document.styleSheets ) {
			if ( !ss.href ) {
				let oid = "~~" + (""+Math.random()).substr(2) + (""+Math.random()).substr(2) + (""+Math.random()).substr(2);
				ss.ownerNode.dataset.hoardStylesheet = oid;
			}
		}


		let contents = formatDTD(document.doctype) + "\n" + document.body.parentNode.outerHTML;
		let doc = new Document();
		let rootNode = doc.importNode(document.documentElement, true)
		doc.append(rootNode);

		for ( let h of siteHooks ) {
			if ( h.re.test(location.href) ) {
				h.fn(doc);
			}
		}



		rm(doc.querySelectorAll("noscript"));
		rm(doc.querySelectorAll("object"));
		rm(doc.querySelectorAll("iframe"));
		for ( let sc of doc.scripts ) {
			rm(sc);
		}
		rm(doc.querySelectorAll("script"));

		let icon_id = null;

		let imageAttachments = {};
		let attachImg = async (image_url, relative_to = location.href) => {
			if ( imageAttachments.hasOwnProperty(image_url) ) {
				return imageAttachments[image_url];
			}
			console.log("getting image url ", image_url);

			let att_id = null;
			let url = new URL(image_url, relative_to);

			if ( url.origin !== location.origin ) {
				att_id = await postDoc("api/proxy-attachment", {url: url.toString()});
				if ( !att_id.filename ) {
					console.error(att_id);
				}
				imageAttachments[image_url] = att_id.filename;
				return att_id.filename;
			}

			let blob = await fetch(url);
			blob = await blob.blob();
			let fileExt = blob.type;

			if ( blob.type.substr(0,6) == "image/" ) {
				fileExt = blob.type.substr(6);
				if ( fileExt == "svg+xml" ) {
					fileExt = "svg";
				} else if ( fileExt == "vnd.microsoft.icon" ) {
					fileExt = "ico";
				}
			} else if ( blob.type.substr(0,5) == "font/" ) {
				fileExt = blob.type.substr(5);
			} else {
				throw "unknown mime type '" + blob.type + "'";
			}

			att_id = await postDoc("api/new-attachment", {ext: fileExt});
			if ( !att_id.attachment_id ) {
				console.error(att_id);
			}
			let filename = att_id.filename;
			att_id = att_id.attachment_id;
			imageAttachments[image_url] = filename;

			await uploadFile(BASE_URL + "api/upload-attachment", "attachment", filename, blob, {
				"txid": txid,
				"att_id": att_id,
			});

			return imageAttachments[image_url];
		};
		let tryAttach = async (u, rt = location.href) => {
			if ( u != "" && u.substr(0,5) != "data:" ) {
				try {
					let s = await attachImg(u, rt)
					return s;
				} catch ( _e ) { console.error("error attaching image", u, _e); }
			}
		};



		for ( let img of doc.images ) {
			// Disable lazy loading
			for ( let k of ["src","srcset"] ) {
				if ( k in img.dataset ) {
					img[k] = img.dataset[k];
					delete img.dataset[k];
				}
			}

			let i = await tryAttach(img.src);
			if ( i ) {
				img.src = i;
			}
			let srcset = [];
			for ( let srcs of parseSrcset(img) ) {
				let i = await tryAttach(srcs.url);
				if ( i ) {
					srcs.url = i;
				}
				srcset.push(`${srcs.url} ${srcs.spec}`);
			}
			img.srcset = srcset.join(", ");
		}


		let attachCSSRule = async (rule, stylesheet_url) => {
			if ( rule instanceof CSSImportRule ) {
				let imp = await attachCSS(rule.styleSheet);
				// HACK: reference attachments local to attached style sheets
				if ( imp.slice(0,4) == "att/" ) { imp = imp.slice(4); }
				return `@import url("${imp}");\n`;
			} else if ( rule instanceof CSSStyleRule ) {
				let resetStyle = {};
				for ( let k of rule.style ) {
					if ( k == "background-image" || k == "list-style-image" || k == "content" || k == "cursor" ) {
						let m = rule.style[k].match(/^url\(\s*\"([^\"]+)\"\s*\)/)
						if ( m ) {
							console.log("attaching", m);
							resetStyle[k] = rule.style[k];
							let s = await tryAttach(m[1], stylesheet_url);
							if ( s ) {
								// HACK: reference attachments local to attached style sheets
								if ( s.slice(0,4) == "att/" ) { s = s.slice(4); }

								//rule.style[k] = rule.style[k].replace(m[0], s);
								rule.style[k] = `url("${s}")`;
							}
							console.log("attached ", s, rule.cssText);
						}
					}
				}
				let rv = rule.cssText + "\n";
				// HACK: reset the style rule to its previous value, so there are no requests
				// for attached files to the origin server
				for ( let k in resetStyle ) {
					if ( resetStyle.hasOwnProperty(k) ) {
						rule.style[k] = resetStyle[k];
					}
				}

				return rv;
			} else if ( rule instanceof CSSMediaRule ) {
				let rv = `@media ${rule.conditionText} {\n`;
				for ( let rr of rule.cssRules ) {
					rv += "    " + await attachCSSRule(rr, stylesheet_url);
				}
				return rv + `}\n`;
			} else if ( rule instanceof CSSFontFaceRule ) {
				// HACK: there's no API-driven way to do this, but maybe there's something nicer than regexes
				let rv = rule.cssText;
				let m = rv.match(/src:\s*((url\(("([^\"]*)"|([^\)]))\)\s*format\(([^\)]*)\),?\s*|local\(\s*"[^\"]+"\s*\),?\s*)*)\s*(;|\})/);
				if ( m ) {
					let srcset = m[1];
					// src: url(foo) format(bar), url(baz) format(qux)${m[6]}`;
					let replacement = [];

					while ( !!srcset && srcset.length > 0 ) {
						if ( srcset.slice(0,4) == "url(" ) {
							let fontFile, format;
							srcset = splitFirst(srcset, "(")[1].trim();
							srcset = splitFirst(srcset, "\"")[1];
							[fontFile, srcset] = splitFirst(srcset, "\"");
							srcset = splitFirst(srcset, ")")[1].trim();
							[format, srcset] = splitFirst(srcset, ",");
							srcset = srcset.trim();
							let s = await tryAttach(fontFile, stylesheet_url);
							if ( s ) {
								if ( s.slice(0,4) == "att/" ) { s = s.slice(4); }
								fontFile = s;
								replacement.push(`url("${fontFile}") ${format}`);
							}
						} else if ( srcset.slice(0,6) == "local(" ) {
							let localFont;
							srcset = splitFirst(srcset, "(")[1].trim();
							srcset = splitFirst(srcset, "\"")[1];
							[localFont, srcset] = splitFirst(srcset, "\"");
							srcset = splitFirst(srcset, ",")[1].trim();
							replacement.push(`local("${localFont}")`);
						} else {
							let wtf;
							[wtf, srcset] = splitFirst(srcset, ",");
							console.error(`unknown font source type '${wtf}'`);
							srcset = srcset.trim();
						}
					}
					replacement = "src: " + replacement.join(", ") + m[7];

					rv = rv.replace(m[0], replacement);
				}
				return rv + "\n";
			} else {
				return rule.cssText + "\n";
			}
		};

		let cssAttachments = {};
		let attachCSS = async (stylesheet) => {
			const stylesheet_href = stylesheet.href;
			let att_key = stylesheet_href;
			let att_id = null, filename = null;
			let url = {origin: false};

			if ( !stylesheet_href ) {
				let ownernode = stylesheet.ownerNode;
				if ( !("hoardStylesheet" in ownernode.dataset) ) {
					return null;
				}
				att_key = ownernode.dataset.hoardStylesheet;
			} else {
				url = new URL(stylesheet_href, location.href);
				att_key = url.toString();
			}

			if ( cssAttachments.hasOwnProperty(att_key) ) {
				return cssAttachments[att_key];
			}
			console.log("Attaching CSS ", att_key);

			if ( !!stylesheet_href && url.origin !== location.origin ) {
				att_id = await postDoc("api/proxy-attachment", {url: stylesheet_href});
				filename = att_id.filename;
				att_id = att_id.attachment_id;
				cssAttachments[att_key] = filename;

				style_cnt = await postDoc("api/download-attachment", {att_id: att_id}, "text");
				stylesheet = new CSSStyleSheet();
				stylesheet.replaceSync(style_cnt);
			} else {
				att_id = await postDoc("api/new-attachment", {ext: "css"});
				filename = att_id.filename;
				att_id = att_id.attachment_id;
				cssAttachments[att_key] = filename;
			}


			let attcnt = "";

			try {
				for ( let rule of stylesheet.cssRules ) {
					attcnt += await attachCSSRule(rule, stylesheet_href || location.href);
				}
			} catch ( _e ) { console.error(_e); }

			await uploadFile(BASE_URL + "api/upload-attachment", "attachment", filename, attcnt, {
				"txid": txid,
				"att_id": att_id,
			});

			return cssAttachments[att_key];
		}

		for ( let stylesheet of document.styleSheets ) {
			await attachCSS(stylesheet);
		}

		for ( let link of doc.querySelectorAll("link") ) {
			if ( link.rel == "stylesheet" ) {
				let href = (new URL(link.href, location.href)).toString();
				if ( cssAttachments.hasOwnProperty(href) ) {
					link.href = cssAttachments[href];
				} else {
					link.rel = "defunct-stylesheet";
					console.error("css link not found", link.href, cssAttachments);
				}
			} else if ( link.rel == "dns-prefetch" || link.rel == "preconnect" ) {
				rm(link);
			} else if ( link.rel == "preload" || link.rel == "amphtml" ) {
				rm(link);
			} else if ( /site-verification/.test(link.rel) ) {
				rm(link);
			} else if ( link.rel == "icon" || link.rel == "apple-touch-icon" || link.rel == "shortcut icon" || /^msapplication-/.test(link.rel) ) {
				let icon = await tryAttach(link.href);
				if ( icon ) {
					link.href = icon;
					if ( !icon_id || link.rel == "apple-touch-icon" ) {
						icon_id = icon;
					}
				} else {
					console.error("attaching icon failed", link.href, link);
				}
			}
		}
		for ( let st of doc.querySelectorAll("style") ) {
			let oid = st.dataset.hoardStylesheet;
			if ( cssAttachments.hasOwnProperty(oid) ) {
				let link = doc.createElement("link");
				link.setAttribute("rel", "stylesheet");
				link.setAttribute("href", cssAttachments[oid]);
				doc.head.appendChild(link);
				rm(st);
			}
		}

		for ( let meta of doc.querySelectorAll("meta") ) {
			let mname = meta.name.toLowerCase();
			let heqv = meta.httpEquiv.toLowerCase();
			let prop = (meta.getAttribute("property") || "").toLowerCase();

			if ( /site-verification/.test(mname) ) {
				rm(meta);
			} else if ( heqv == "origin-trial" || mname == "robots" ) {
				rm(meta);
			} else if ( mname == "icon" || mname == "apple-touch-icon" || mname == "shortcut icon" || /^msapplication-/.test(mname) || /:image$/.test(mname) || /:image$/.test(prop) ) {
				let img = await tryAttach(meta.content);
				if ( img ) {
					meta.content = img;
				} else {
					console.error("attaching meta image failed", meta.content, meta);
				}
			}
		}


		// Remove scripting attributes (onclick, onload, onerror, etc)
		let rmAttr = {
			"data-ga": 1,
		};
		for ( let elt of doc.querySelectorAll("*") ) {
			if ( elt.attributes.length == 0 ) {
				continue;
			}
			let found = false;
			for ( let att of elt.attributes ) {
				if ( att.name in rmAttr || att.name.substr(0,2) == "on" ) {
					rmAttr[att.name] = 1;
					found = true;
				}
			}
			if ( found ) {
				for ( let att in rmAttr ) {
					elt.removeAttribute(att);
				}
			}
		}
		for ( let elt of doc.body.querySelectorAll("*") ) {
			if ( elt.nodeName == "style" ) {
				continue;
			}
			if ( elt.nodeName == "svg" || elt.nodeName == "g" ) {
				continue;
			}
			if ( window.getComputedStyle(elt).getPropertyValue("display") == "none" ) {
				// console.log("maybe remove this one?", elt)
				//rm(elt);
				elt.style.display = "none";
			}
		}

		contents = formatDTD(document.doctype) + "\n" + doc.body.parentNode.outerHTML;


		await uploadFile(BASE_URL + "api/upload-draft", "document", document.title + ".html", contents, {
			"txid": txid,
			"doc_id": doc_id,
		});

		if ( !icon_id ) {
			icon_id = await tryAttach("/favicon.ico");
		}
		if ( icon_id ) {
			let m = icon_id.match(/^(att\/)?(\w+)(\.\w+)?/); // HACK
			if ( m ) {
				icon_id = m[2];
			}
		}

		let finalize = await postDoc("api/finalize-draft", {
			doc_title: document.title,
			doc_author: "", // TODO
			icon_id: icon_id,
			log_message: "Saved page from web extension",
		});
		if ( !finalize.ok ) { console.error(finalize); }

		return {
			success: true,
			document_id: doc_id,
			full_url: (new URL("documents/view/g" + doc_id + "/", BASE_URL)).toString(),
		};
	}


	// Listen for messages from the background script.
	browser.runtime.onMessage.addListener(async (message) => {
		console.log("Got message", message);
		if (message.command === "flatten") {
			return await flatten(message.txid, message.doc_id);
		}
	});

})();

