/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { History } from './History';
import type { Mutations } from './Mutations';
export type Document = {
    id: string;
    model: string;
    version?: number;
    history?: History;
    val: any;
    mut?: Mutations;
    status?: Record<string, any>;
};

