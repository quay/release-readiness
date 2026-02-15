import { githubCommitUrl } from "../utils/links";

export default function GitShaLink({
	component,
	sha,
	gitUrl,
}: {
	component: string;
	sha: string;
	gitUrl?: string;
}) {
	if (!sha) return null;
	const display = sha.substring(0, 12);
	const url = gitUrl
		? `${gitUrl.replace(/\.git$/, "").replace(/\/+$/, "")}/commit/${sha}`
		: githubCommitUrl(component, sha);
	if (url) {
		return (
			<a href={url} target="_blank" rel="noopener noreferrer">
				<code>{display}</code>
			</a>
		);
	}
	return <code>{display}</code>;
}
