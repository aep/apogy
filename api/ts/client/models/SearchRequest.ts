/* generated using openapi-typescript-codegen -- do not edit */
/* istanbul ignore file */
/* tslint:disable */
/* eslint-disable */
import type { Filter } from './Filter';
export type SearchRequest = {
    model: string;
    filters?: Array<Filter>;
    cursor?: string;
    limit?: number;
};

