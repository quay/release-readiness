import {
	Button,
	EmptyState,
	EmptyStateActions,
	EmptyStateBody,
	EmptyStateFooter,
	Title,
} from "@patternfly/react-core";
import { Component } from "react";

interface Props {
	children: React.ReactNode;
}

interface State {
	error: Error | null;
}

export default class ErrorBoundary extends Component<Props, State> {
	state: State = { error: null };

	static getDerivedStateFromError(error: Error): State {
		return { error };
	}

	render() {
		if (this.state.error) {
			return (
				<EmptyState>
					<Title headingLevel="h2" size="lg">
						Something went wrong
					</Title>
					<EmptyStateBody>{this.state.error.message}</EmptyStateBody>
					<EmptyStateFooter>
						<EmptyStateActions>
							<Button
								variant="primary"
								onClick={() => this.setState({ error: null })}
							>
								Try again
							</Button>
						</EmptyStateActions>
					</EmptyStateFooter>
				</EmptyState>
			);
		}
		return this.props.children;
	}
}
