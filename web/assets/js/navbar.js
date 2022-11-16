
(async () => {
	let prp = document.getElementById("nav-smoel");
	let profileMenu = document.getElementById("profile-menu")
	prp.addEventListener("click", () => {
		if ( profileMenu.open ) {
			profileMenu.close();
		} else {
			profileMenu.showModal();
		}
	})
})()

