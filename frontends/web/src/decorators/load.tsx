/**
 * Copyright 2018 Shift Devices AG
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

import { h, Component, RenderableProps, ComponentConstructor, FunctionalComponent } from 'preact';
import { Endpoint, EndpointsObject, EndpointsFunction } from './endpoint';
import { apiGet } from '../utils/request';
import { KeysOf } from '../utils/types';

// Stores whether to log the time needed for individual API calls.
const logPerformance = false;

// The counter is used to measure the time needed for individual API calls.
let logCounter = 0;

/**
 * Loads API endpoints into the props of the component that uses this decorator.
 * 
 * @param endpointsObjectOrFunction - The endpoints that should be loaded to their respective property name.
 * @param renderOnlyOnceLoaded - Whether the decorated component shall only be rendered once all endpoints are loaded.
 * @return A function that returns the higher-order component that loads the endpoints into the props of the decorated component.
 */
export function load<LoadedProps, ProvidedProps = {}>(
    endpointsObjectOrFunction: EndpointsObject<LoadedProps> | EndpointsFunction<ProvidedProps, LoadedProps>,
    renderOnlyOnceLoaded: boolean = true, // Use false only if all loaded props are optional!
) {
    return function decorator(
        WrappedComponent: ComponentConstructor<LoadedProps & ProvidedProps> | FunctionalComponent<LoadedProps & ProvidedProps>,
    ) {
        return class Load extends Component<ProvidedProps & Partial<LoadedProps>, LoadedProps> {
            private determineEndpoints(): EndpointsObject<LoadedProps> {
                if (typeof endpointsObjectOrFunction === 'function') {
                    return endpointsObjectOrFunction(this.props);
                }
                return endpointsObjectOrFunction;
            }

            private loadEndpoint(key: keyof LoadedProps, endpoint: Endpoint): void {
                logCounter += 1;
                const timerID = endpoint + ' ' + logCounter;
                if (logPerformance) { console.time(timerID); }
                apiGet(endpoint).then(object => {
                    this.setState({ [key]: object } as Pick<LoadedProps, keyof LoadedProps>);
                    if (logPerformance) { console.timeEnd(timerID); }
                });
            }

            private endpoints: EndpointsObject<LoadedProps>;

            private loadEndpoints(): void {
                const oldEndpoints = this.endpoints;
                const newEndpoints = this.determineEndpoints();
                // Load the endpoints that were different or undefined before.
                for (const key of Object.keys(newEndpoints) as KeysOf<LoadedProps>) {
                    if (oldEndpoints == null || newEndpoints[key] !== oldEndpoints[key]) {
                        this.loadEndpoint(key, newEndpoints[key] as Endpoint);
                    }
                }
                if (oldEndpoints != null) {
                    // Remove endpoints that no longer exist from the state.
                    for (const key of Object.keys(oldEndpoints) as KeysOf<LoadedProps>) {
                        if (newEndpoints[key] === undefined) {
                            this.setState({ [key]: undefined as any} as Pick<LoadedProps, keyof LoadedProps>);
                        }
                    }
                }
                this.endpoints = newEndpoints;
            }

            public componentDidMount(): void {
                this.loadEndpoints();
            }

            public componentDidUpdate(): void {
                this.loadEndpoints();
            }

            private allEndpointsLoaded(): boolean {
                if (this.endpoints == null) { return false; }
                for (const key of Object.keys(this.endpoints) as KeysOf<LoadedProps>) {
                    if (this.state[key] === undefined) {
                        return false;
                    }
                }
                return true;
            }
            
            public render(props: RenderableProps<ProvidedProps & Partial<LoadedProps>>, state: LoadedProps): JSX.Element | null {
                if (renderOnlyOnceLoaded && !this.allEndpointsLoaded()) { return null; }
                return <WrappedComponent {...state} {...props as any} />; // This order allows the subscribe decorator (and others) to override the loaded endpoints with properties.
            }
        };
    };
}
