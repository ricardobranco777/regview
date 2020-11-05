"""
Docker Registry module
"""

import fnmatch
import logging
import sys

from requests.exceptions import RequestException

from .docker_registry import DockerRegistry, Tag


class DockerHub(DockerRegistry):
    """
    Class to implement Docker Registry methods
    """
    _token = None

    @property
    def token(self):
        """
        Token for auth
        """
        if self._token is None:
            url = "https://hub.docker.com/v2/users/login/"
            auth = {"username": self.session.auth.username, "password": self.session.auth.password}
            try:
                got = self.session.post(url, data=auth)
                got.raise_for_status()
            except RequestException as err:
                logging.error("%s: %s", url, err)
                sys.exit(1)
            self._token = got.json()['token']
        return self._token

    # Note: Test adding "?page_size=1" to url
    def _get_paginated(self, url, string=None, **kwargs):
        """
        Get paginated results
        """
        items = []
        while True:
            try:
                got = self.session.get(url, **kwargs)
                got.raise_for_status()
            except RequestException as err:
                logging.error("%s: %s", url, err)
                sys.exit(1)
            data = got.json()
            items.extend(data['results'])
            if not data['next']:
                break
            url = data['next']
        return items

    def get_namespaces(self):
        """
        Get namespaces
        """
        url = "https://hub.docker.com/v2/repositories/namespaces/"
        try:
            got = self.session.get(url, headers={"Authorization": f"JWT {self.token}"})
            got.raise_for_status()
        except RequestException as err:
            logging.error("%s: %s", url, err)
            sys.exit(1)
        data = got.json()
        return data['namespaces']

    def get_repos(self, pattern=None):
        """
        Get repositories
        """
        repos = []
        headers = {"Authorization": f"JWT {self.token}"}
        for namespace in self.get_namespaces():
            url = f"https://hub.docker.com/v2/repositories/{namespace}/"
            repos.extend([
                f"{namespace}/{_['name']}"
                for _ in self._get_paginated(url, namespace, headers=headers)])
        if repos and pattern:
            return fnmatch.filter(repos, pattern)
        return repos

    def get_tags(self, repo, pattern=None):
        """
        Get tags
        """
        url = f"https://hub.docker.com/v2/repositories/{repo}/tags/"
        headers = {"Authorization": f"JWT {self.token}"}
        tags = [Tag(info['name'], info=info) for info in self._get_paginated(url, headers=headers)]
        if tags and pattern:
            return fnmatch.filter(tags, pattern)
        return tags
