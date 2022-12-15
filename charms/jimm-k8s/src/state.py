#!/usr/bin/env python3
# Copyright 2022 Canonical Ltd.
#
# This program is free software: you can redistribute it and/or modify
# it under the terms of the GNU General Public License version 3, as
# published by the Free Software Foundation.
#
# This program is distributed in the hope that it will be useful, but
# WITHOUT ANY WARRANTY; without even the implied warranties of
# MERCHANTABILITY, SATISFACTORY QUALITY, or FITNESS FOR A PARTICULAR
# PURPOSE.  See the GNU General Public License for more details.
#
# You should have received a copy of the GNU General Public License
# along with this program. If not, see <http://www.gnu.org/licenses/>.

from ops.model import Application, Model, Relation


class RelationNotReadyError(Exception):
    pass


class PeerRelationState:
    """RelationState uses the peer relation to store the state of the charm."""

    def __init__(
        self,
        model: Model,
        app: Application,
        relation_name: str,
        defaults: dict[str:str] = None,
    ):
        self._model = model
        self._app = app
        self._relation_name = relation_name

        if defaults:
            relation = self._model.get_relation(relation_name)
            if not relation:
                raise RelationNotReadyError
            else:
                relation.data[self._app].update(defaults)

    def _get_relation(self) -> Relation:
        relation = self._model.get_relation(self._relation_name)
        return relation

    def set(self, key: str, value: str) -> None:
        relation = self._get_relation()
        if not relation:
            raise RelationNotReadyError
        else:
            relation.data[self._app].update({key: value})

    def unset(self, *keys) -> None:
        relation = self._get_relation()
        if not relation:
            raise RelationNotReadyError
        else:
            for key in keys:
                relation.data[self._app].pop(key)

    def get(self, key: str) -> str:
        relation = self._get_relation()
        if not relation:
            return ""
        else:
            return relation.data[self._app].get(key, "")
