const $ = <T extends Element = HTMLElement>(
	selector: string,
	root: ParentNode = document,
) => root.querySelector<T>(selector);
const $$ = <T extends Element = HTMLElement>(
	selector: string,
	root: ParentNode = document,
) => Array.from(root.querySelectorAll<T>(selector));

// Every interaction is local to the demo. External actions remain ordinary links.
function showDialog(content: HTMLElement, label: string) {
	const dialog = document.createElement("dialog");
	dialog.className = "demo-dialog";
	dialog.setAttribute("aria-label", label);
	const close = document.createElement("button");
	close.className = "dialog-close";
	close.textContent = "×";
	close.setAttribute("aria-label", "Close");
	close.addEventListener("click", () => dialog.close());
	dialog.append(close, content);
	dialog.addEventListener("click", (event) => {
		if (event.target === dialog) dialog.close();
	});
	dialog.addEventListener("close", () => {
		dialog.remove();
	});
	document.body.append(dialog);
	dialog.showModal();
}

// Accessible FAQ accordion; preserve the original expanded first answer.
$$<HTMLButtonElement>("#faqs button[aria-controls]").forEach((button) => {
	button.addEventListener("click", () => {
		const isOpen = button.getAttribute("aria-expanded") === "true";
		const panel = document.getElementById(
			button.getAttribute("aria-controls")!,
		);
		button.setAttribute("aria-expanded", String(!isOpen));
		$("svg", button)?.classList.toggle("rotate-45", !isOpen);
		if (panel) {
			panel.hidden = isOpen;
			panel.style.height = isOpen ? "0px" : "auto";
			panel.style.opacity = isOpen ? "0" : "1";
		}
	});
});

const themeButtons = $$<HTMLButtonElement>("footer [data-theme-toggle]");
function setTheme(dark: boolean) {
	document.documentElement.classList.toggle("dark", dark);
	document.documentElement.classList.toggle("light", !dark);
	for (const button of themeButtons) {
		button.setAttribute(
			"aria-label",
			`Switch to ${dark ? "light" : "dark"} mode`,
		);
	}
}
for (const button of themeButtons) {
	button.addEventListener("click", () => {
		const dark = !document.documentElement.classList.contains("dark");
		setTheme(dark);
		try {
			localStorage.setItem("grok-bot-theme", dark ? "dark" : "light");
		} catch {
			/* Storage is optional. */
		}
	});
}
try {
	if (localStorage.getItem("grok-bot-theme") === "dark") setTheme(true);
} catch {
	/* Storage is optional. */
}

$("[data-mobile-menu]")?.addEventListener("click", () => {
	const menu = document.createElement("nav");
	menu.className = "mobile-navigation";
	for (const link of $$("header nav ul a")) {
		const clone = link.cloneNode(true) as HTMLAnchorElement;
		clone.addEventListener("click", () => {
			menu.closest("dialog")?.close();
		});
		menu.append(clone);
	}
	showDialog(menu, "Navigation");
});

$$<HTMLButtonElement>("[data-copy-command]").forEach((button) => {
	button.addEventListener("click", async () => {
		const status = $(".quickstart-status");
		const code = button.closest(".quickstart-command")?.querySelector("code");
		const command = (
			button.dataset.copyCommand ||
			code?.innerText ||
			""
		).trim();
		try {
			await navigator.clipboard.writeText(command);
			for (const other of $$<HTMLButtonElement>("[data-copy-command]")) {
				if (other !== button) other.textContent = "Copy";
			}
			button.textContent = "Copied";
			if (status)
				status.textContent = button.dataset.copyStatus || "Prompt copied.";
		} catch {
			if (status) status.textContent = "Select and copy the text above.";
		}
	});
});
